# Store + Emailer Seams — Implementation Plan

Spec: `docs/superpowers/specs/2026-07-05-store-emailer-seams-design.md`
Branch: `refactor/store-emailer-seams`

## Global Constraints

- Consumer-side interfaces live in `cmd/api` (new file `cmd/api/stores.go`).
- Interface method sets are copied **verbatim** from the concrete types in
  `internal/data` and `internal/mailer` — do NOT change any signature in those
  packages (except removing `MovieModel.pool` in Task 2).
- `Emailer.Send` signature: `Send(recipient, templateFile string, data any) error`.
- `data.Models` and `data.NewModels` remain as the composition root in `main()`.
- No new features, endpoints, or mailer-internal changes.
- Each task ends green: `go build ./...`, `go vet ./...`, `go test ./...`.
- Data-layer (DB-gated) tests run with `GREENLIGHT_TEST_DB_DSN` set to the
  compose `postgres-test` instance (port 5433).

## Task 1 — Introduce seams; rewire `application`

**Files:** `cmd/api/stores.go` (new), `cmd/api/main.go`, `cmd/api/movies.go`,
`cmd/api/tokens.go`, `cmd/api/users.go`, `cmd/api/middlewares.go`.

**Steps:**

1. Create `cmd/api/stores.go` declaring `MovieStore`, `UserStore`, `TokenStore`,
   `PermissionStore`, `Emailer` (exact method sets from the spec). Import
   `data "github.com/vancanhuit/greenlight/internal/data"` and `time`.
2. In `main.go`, change `application` fields: remove `models data.Models` and
   `mailer mailer.Mailer`; add `movies MovieStore`, `users UserStore`,
   `tokens TokenStore`, `permissions PermissionStore`, `mailer Emailer`.
3. In `main()`, build `models := data.NewModels(db)` then assign
   `movies: models.Movies`, `users: models.Users`, `tokens: models.Tokens`,
   `permissions: models.Permissions`, `mailer: mailer.New(...)`.
4. Rewrite call sites:
   - `middlewares.go`: `app.models.Users.GetForToken` → `app.users.GetForToken`;
     `app.models.Permissions.GetAllForUser` → `app.permissions.GetAllForUser`.
   - `movies.go`: `app.models.Movies.` → `app.movies.` (5 sites: Insert, Get ×2,
     Update, Delete, GetAll).
   - `tokens.go`: `app.models.Users.GetByEmail` → `app.users.GetByEmail`;
     `app.models.Tokens.New` → `app.tokens.New`.
   - `users.go`: `app.models.Users.Insert` → `app.users.Insert`;
     `app.models.Permissions.AddForUser` → `app.permissions.AddForUser`;
     `app.models.Tokens.New` → `app.tokens.New`;
     `app.models.Users.GetForToken` → `app.users.GetForToken`;
     `app.models.Users.Update` → `app.users.Update`;
     `app.models.Tokens.DeleteAllForUser` → `app.tokens.DeleteAllForUser`;
     `app.mailer.Send` stays.
5. Keep the `mailer` package import in `main.go` (still used by `mailer.New`).
   Remove the `data` import from files that no longer reference the package if
   unused (check `movies.go`/`users.go` still use `data.X` for constants/types —
   they do; keep).

**Done when:** `go build ./...`, `go vet ./...`, `go test ./...` all pass.
Behavior unchanged (pure indirection).

## Task 2 — Route `GetAll` through sqlc; drop `MovieModel.pool`

**Files:** `internal/data/queries/movies.sql`, `internal/data/sqlc/*` (generated),
`internal/data/movies.go`, `internal/data/models.go`.

**Steps:**

1. Add a `ListMovies :many` query to `queries/movies.sql`. Reproduce the current
   `GetAll` SQL with named params and a whitelisted CASE `ORDER BY`:

   ```sql
   -- name: ListMovies :many
   SELECT count(*) OVER() AS total, id, created_at, title, year, runtime, genres, version
   FROM movies
   WHERE (to_tsvector('simple', title) @@ plainto_tsquery('simple', @title) OR @title = '')
     AND (genres @> @genres::text[] OR @genres::text[] = '{}')
   ORDER BY
     CASE WHEN @sort_column::text = 'id'      AND @sort_direction::text = 'ASC'  THEN id      END ASC,
     CASE WHEN @sort_column::text = 'id'      AND @sort_direction::text = 'DESC' THEN id      END DESC,
     CASE WHEN @sort_column::text = 'title'   AND @sort_direction::text = 'ASC'  THEN title   END ASC,
     CASE WHEN @sort_column::text = 'title'   AND @sort_direction::text = 'DESC' THEN title   END DESC,
     CASE WHEN @sort_column::text = 'year'    AND @sort_direction::text = 'ASC'  THEN year    END ASC,
     CASE WHEN @sort_column::text = 'year'    AND @sort_direction::text = 'DESC' THEN year    END DESC,
     CASE WHEN @sort_column::text = 'runtime' AND @sort_direction::text = 'ASC'  THEN runtime END ASC,
     CASE WHEN @sort_column::text = 'runtime' AND @sort_direction::text = 'DESC' THEN runtime END DESC,
     id
   LIMIT @page_limit OFFSET @page_offset;
   ```

   Adjust param names if `sqlc generate` reports issues; keep behaviour identical
   to the current raw query. `id` tiebreak preserves stable ordering.
2. Run `sqlc generate`. Confirm a `ListMovies` method and params/row structs
   appear in `internal/data/sqlc/movies.sql.go`.
3. Rewrite `MovieModel.GetAll` to call `m.q.ListMovies(ctx, db.ListMoviesParams{...})`
   with `Title: title`, `Genres: genres`, `SortColumn: filters.sortColumn()`,
   `SortDirection: filters.sortDirection()`, `PageLimit: int32(filters.limit())`,
   `PageOffset: int32(filters.offset())` (match generated field names/types).
   Map returned rows to `[]*data.Movie`; read `total` from the first row (0 when
   empty) and build `Metadata` via `calculateMetadata`.
4. Remove the `pool *pgxpool.Pool` field from `MovieModel`; in `data.NewModels`
   change `Movies: MovieModel{q: q, pool: pool}` to `Movies: MovieModel{q: q}`.
   Remove now-unused imports (`fmt`, `pgxpool`) from `movies.go` if no longer
   referenced.

**Done when:** `go build ./...`, `go vet ./...` pass; data-layer tests pass
against the test DB, including `GetAll` sort/filter/pagination coverage matching
prior behaviour. If existing `GetAll` tests are thin, add cases covering each
safelisted sort column (both directions), genre filter, title filter, and
pagination metadata.

## Task 3 — Fakes + broad handler test suite

**Files:** `cmd/api/*_test.go` (new), e.g. `cmd/api/fakes_test.go`,
`cmd/api/movies_test.go`, `cmd/api/users_test.go`, `cmd/api/tokens_test.go`.

**Steps:**

1. Add in-memory fakes in `cmd/api` test files implementing `MovieStore`,
   `UserStore`, `TokenStore`, `PermissionStore`, and a recording `Emailer`
   (map-backed; captures sends as `{recipient, template, data}`). Provide a
   helper that builds an `application` wired with fakes + a discarding
   `jsonlog.Logger` and a minimal `config`.
2. Reuse existing route wiring (`app.routes()`) with `httptest.NewRecorder` +
   `httptest.NewRequest`; drive requests through the real router so middleware
   and envelopes are exercised. For permission-gated movie routes, seed a fake
   user + token + permissions so `authenticate`/`requirePermissions` pass.
3. Test cases (assert status, envelope JSON, error mapping):
   - **Movies**: create (201 + Location), show (200; 404 unknown; 404 bad id),
     list (200 with sort/filter/pagination via fake), update (200; 404; 409 edit
     conflict via fake returning `data.ErrEditConflict`), delete (200; 404).
     Validation failure returns 422 with field errors.
   - **Users**: register (201; permission `movies:read` granted; welcome email
     recorded by fake `Emailer`; duplicate email → 422), activate (200 on valid
     token; 422/404 on bad token).
   - **Tokens**: authentication (201 token on valid credentials; 401 on bad
     password; 422 on invalid input).

**Done when:** `go test ./cmd/api/...` passes with the new suite; `go test ./...`
green overall.

## Verification (whole branch)

- `go build ./...`, `go vet ./...` clean.
- `go test ./...` green (handler tests always; data tests against test DB).
- Diff contains no behaviour change beyond the seam + `GetAll` sqlc routing.
- Old raw-SQL `GetAll` and `MovieModel.pool` fully removed (grep to confirm).
