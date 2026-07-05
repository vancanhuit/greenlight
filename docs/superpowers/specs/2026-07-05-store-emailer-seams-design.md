# Store + Emailer Seams — Design

Date: 2026-07-05
Branch: `refactor/store-emailer-seams`
Source: architecture review (`/tmp/architecture-review-*.html`) — deep-module refactor.

## Goal

Turn the shallow HTTP handler layer into a deep, testable one by placing seams
between the handlers and infrastructure (Postgres, SMTP). Today handlers hold
concrete `data.Models` and `mailer.Mailer`, so the only test surface is a live
database plus a live SMTP server — result: zero handler tests. Introduce small
consumer-side interfaces the handlers depend on, make the existing concrete
types adapters, and add in-memory fakes so the whole request/response layer
becomes testable.

Scope covers all three review candidates:

1. **Store + Emailer seams** (strong) — the core change.
2. **`GetAll` through sqlc** (worth exploring) — remove `MovieModel`'s dual
   dependency so all models are uniform behind the `MovieStore` seam.
3. **`Emailer` interface** (speculative) — folded into candidate 1; mailer
   internals (client-per-send, retry loop) left untouched.

## Decisions (locked)

| # | Decision | Choice |
|---|----------|--------|
| Scope | Which candidates | All three (1 + 2 + 3) |
| Store shape | Grouped vs separate | **Separate** interfaces; each handler depends only on what it needs |
| Interface location | Consumer vs producer | **Consumer side** (`cmd/api`); concrete types satisfy implicitly |
| `GetAll` | sqlc vs raw | **sqlc** with CASE-based `ORDER BY`; drop `pool` field |
| Mailer | How far | **Interface only**; no internal client/retry change |
| Tests | How much now | **Broad** handler suite (movies CRUD, users register/activate, tokens auth) |

## Architecture

### Seams (consumer-side interfaces in `cmd/api`)

New file `cmd/api/stores.go` declares five interfaces. Method sets copied
verbatim from the existing concrete types (`internal/data`, `internal/mailer`),
so the concrete types satisfy them implicitly — no change to `internal/data`
or `internal/mailer` signatures.

```go
type MovieStore interface {
    Insert(movie *data.Movie) error
    Get(id int64) (*data.Movie, error)
    GetAll(title string, genres []string, filters data.Filters) ([]*data.Movie, data.Metadata, error)
    Update(movie *data.Movie) error
    Delete(id int64) error
}

type UserStore interface {
    Insert(user *data.User) error
    GetByEmail(email string) (*data.User, error)
    Update(user *data.User) error
    GetForToken(tokenScope, tokenPlaintext string) (*data.User, error)
}

type TokenStore interface {
    Insert(token *data.Token) error
    DeleteAllForUser(scope string, userID int64) error
    New(userID int64, ttl time.Duration, scope string) (*data.Token, error)
}

type PermissionStore interface {
    GetAllForUser(userID int64) (data.Permissions, error)
    AddForUser(userID int64, codes ...string) error
}

type Emailer interface {
    Send(recipient, templateFile string, data any) error
}
```

### `application` struct

Replace the two concrete fields with five interface fields:

```go
type application struct {
    config      config
    logger      *jsonlog.Logger
    movies      MovieStore
    users       UserStore
    tokens      TokenStore
    permissions PermissionStore
    mailer      Emailer
    wg          sync.WaitGroup
}
```

Wiring in `main()`:

```go
models := data.NewModels(db)
app := application{
    config:      cfg,
    logger:      logger,
    movies:      models.Movies,
    users:       models.Users,
    tokens:      models.Tokens,
    permissions: models.Permissions,
    mailer:      mailer.New(...),
}
```

### Call-site rewrites

`app.models.<Group>.<Method>` becomes `app.<group>.<Method>`:

- `cmd/api/middlewares.go`: `app.models.Users.GetForToken` → `app.users.GetForToken`;
  `app.models.Permissions.GetAllForUser` → `app.permissions.GetAllForUser`
- `cmd/api/movies.go`: `app.models.Movies.*` → `app.movies.*` (Insert, Get ×2, Update, Delete, GetAll)
- `cmd/api/tokens.go`: `app.models.Users.GetByEmail` → `app.users.GetByEmail`;
  `app.models.Tokens.New` → `app.tokens.New`
- `cmd/api/users.go`: `app.models.Users.*` → `app.users.*`;
  `app.models.Permissions.AddForUser` → `app.permissions.AddForUser`;
  `app.models.Tokens.*` → `app.tokens.*`; `app.mailer.Send` unchanged (now `Emailer`)

`data.Models`/`data.NewModels` stay as the composition root used by `main()`.

### `GetAll` via sqlc (candidate 2)

Add a `ListMovies` query to `internal/data/queries/movies.sql` that reproduces
the current behaviour — full-text title filter, genres containment, dynamic
sort, count window, limit/offset — using a whitelisted CASE-based `ORDER BY`
(one CASE expression per sort column + direction, `id` tiebreak). Sort column
and direction arrive as text params; the safelist stays enforced in
`filters.sortColumn()`/`sortDirection()` (unchanged). Each CASE returns a single
column, so branch types stay homogeneous.

Regenerate with `sqlc generate`. Rewrite `MovieModel.GetAll` to call
`m.q.ListMovies(...)`, mapping rows to `[]*data.Movie` and building `Metadata`
from the window count. Drop the `pool *pgxpool.Pool` field from `MovieModel`
and its assignment in `data.NewModels`. `GetAll`'s Go signature is unchanged,
so `MovieStore` is unaffected.

### Fakes + handler tests (candidate 1 payoff, test scope A)

In-memory fakes in `cmd/api` test files satisfy the five interfaces (map-backed
stores; a recording `Emailer`). Broad handler test suite exercising the seam:

- **Movies**: create, show, list (incl. sort/filter), update, delete — success
  and key error paths (404, validation 422, edit conflict).
- **Users**: register (verifies permission grant + welcome email recorded via
  fake `Emailer`), activate.
- **Tokens**: authentication (valid credentials issue token; bad credentials
  rejected).

Assert status codes, JSON envelopes, and error mapping — the locality the seam
unlocks.

## Non-goals

- No change to mailer internals (client-per-send, `time.Sleep` retry loop).
- No change to `data.Models` method signatures or error translation.
- No new features or endpoints.
- No production-gating of auto-migrations (noted in review, out of scope here).

## Testing strategy

- **Handler tests** (`cmd/api`): fakes only, no DB — run everywhere including CI.
- **Data-layer tests** (`internal/data`): DB-gated via `GREENLIGHT_TEST_DB_DSN`
  (compose `postgres-test`, port 5433). `GetAll`/`ListMovies` verified here.
- Gate: `go build ./...`, `go vet ./...`, and `go test ./...` clean; data tests
  green against the test DB.

## Risks

- **CASE `ORDER BY` correctness** — sort must match the current raw query across
  all safelisted columns and both directions. Covered by DB-gated `GetAll` tests.
- **sqlc param typing** — text sort params and `[]string` genres must generate
  correctly for `pgx/v5`; iterate `sqlc generate` until the mapping is clean.
