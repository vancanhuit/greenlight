# Greenlight Refactor: sqlc + goose + mise + Tests — Design

**Date:** 2026-07-03
**Status:** Approved (chat)

## Goal

Refactor the existing `greenlight` JSON API (from *Let's Go Further*) in place,
keeping the same public behaviour and HTTP API, while replacing the data and
tooling layers:

- **DB access:** `database/sql` + `lib/pq` → **sqlc**-generated code on **pgx/v5**.
- **Migrations:** golang-migrate → **goose** (annotated SQL, same `migrations/` dir).
- **Tooling:** `Makefile` → **mise** (`[tools]`, `[env]`, `[tasks]`).
- **Local stacks:** single **`compose.yaml`** with profiles + YAML anchors.
- **Tests:** add unit tests (pure logic) and DB integration tests (docker compose stack).
- **CI:** add GitHub Actions workflows.

Handlers and the HTTP layer stay unchanged. The `internal/data` model wrapper keeps
its public method set; only its internals change.

## Constraints

- Module path: `github.com/vancanhuit/greenlight`.
- Bump `go.mod` to **Go 1.22+** (required by pgx/v5 + sqlc tooling).
- Vendoring stays (book ch19.4): run `go mod vendor` after any dependency change.
- Each work item ships as its own branch → PR → CI → squash-merge → delete branch.
- `main` stays green (compilable) after every PR.

## Architecture

### Tooling — `mise.toml`

- `[tools]`: `go`, `sqlc`, `goose`, `golangci-lint`.
- `[env]`: `GREENLIGHT_DB_DSN`, `_.file = ".env"` for local secrets.
- `[tasks]`: `run`, `build`, `test` (unit), `test:db` (integration), `lint`,
  `sqlc` (generate), `migrate:up`, `migrate:down`, `migrate:new`,
  `db:up` / `db:down` (test compose stack), `dev:up`, `dev:up:https`,
  `certs:setup` (mkcert).
- Delete `Makefile`; update `README.md` to mise commands.

### Local stacks — single `compose.yaml`

- **YAML anchors/fragments:** `x-postgres-base: &pg-base` (image `postgres:16`,
  healthcheck, env), `x-api-base: &api-base` (build + shared env) reused across services.
- **Profiles:**
  - `test` → `postgres-test` (tmpfs volume, isolated DB/port) — used by `mise test:db` and local CI.
  - `dev` → `postgres-dev` + `api` over plain HTTP.
  - `dev-https` → `postgres-dev` + `api` + `caddy` reverse proxy terminating TLS
    with **mkcert** local certs mounted from `./.certs`.
- Selected via `docker compose --profile <name> up`, wrapped by mise tasks.
- `.certs/` is gitignored; mkcert CA install documented in README.

### Migrations — goose

- Convert the 5 existing golang-migrate migration pairs to goose-annotated SQL
  (`-- +goose Up` / `-- +goose Down`), keeping the numeric sequence and the
  `migrations/` directory.
- sqlc reads `migrations/` as its schema source (single source of truth).

### Data access — sqlc + pgx

- `sqlc.yaml`: engine `postgresql`, `sql_package: pgx/v5`, queries in
  `internal/data/queries/*.sql`, schema `migrations/`, generated output
  `internal/data/sqlc` (package `db`).
- Query files: `movies.sql`, `users.sql`, `tokens.sql`, `permissions.sql`.
- `pgxpool.Pool` constructed in `cmd/api/main.go`, passed to `data.NewModels`.

### Data layer wrapper — `internal/data`

- `Models`, `MovieModel`, `UserModel`, `TokenModel`, `PermissionModel` keep their
  **existing public methods** (handlers untouched). Each holds `*db.Queries`
  (and the pool for tx where needed) instead of `*sql.DB`.
- Methods translate sqlc row types ↔ domain structs (`Movie`, `User`, ...).
- Error mapping:
  - `pgx.ErrNoRows` → `ErrRecordNotFound`
  - unique-violation SQLSTATE `23505` → `ErrDuplicateEmail`
  - zero-rows optimistic update → `ErrEditConflict`
- `ValidateMovie` / `ValidateUser` / etc. remain unchanged.
- Postgres `text[]` (genres) uses pgx native `[]string` (no `pq.Array`).

### Tests

- **Unit (no DB):** `internal/validator`, `internal/data` filters, `Runtime`
  marshalling, `cmd/api` helpers, `internal/jsonlog`.
- **DB integration:** `mise test:db` brings up the `test` compose profile;
  a `TestMain` runs `goose up` against the test DB and truncates tables between
  tests; each model is exercised against real Postgres. DB tests are guarded by
  the presence of the test DSN env var, so a plain `go test ./...` runs
  unit-only when no DB is available.

### CI — GitHub Actions (`.github/workflows/ci.yml`)

Triggers on push and pull_request. Jobs:

- **lint:** `gofmt` check, `go vet`, `golangci-lint`/`staticcheck`.
- **test:** Postgres service container → `goose up` → `go test ./...` (unit + DB).
- **audit:** `go mod verify`, vendor consistency (`go mod vendor` + `git diff --exit-code`).

## Rollout — PR sequence (each own branch/PR/CI/squash)

1. mise + `compose.yaml` (profiles/anchors, mkcert) + delete `Makefile` + README.
2. GitHub Actions CI skeleton (lint + audit; test job stubbed).
3. goose migration conversion.
4. sqlc + pgx wiring + **movies** model ported.
5. **users** model ported.
6. **tokens** model ported.
7. **permissions** model ported.
8. Remove `lib/pq` + `database/sql`, re-vendor.
9. Unit tests.
10. DB integration tests + CI test job wired to Postgres service.

## Out of scope

- End-to-end API tests (`httptest.Server`) — not selected.
- testcontainers-go — explicitly replaced by docker compose.
- Deployment chapters (book ch20) and appendices (ch21).
