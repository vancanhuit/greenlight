# Greenlight sqlc + goose + mise Refactor — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Refactor greenlight's data + tooling layers to sqlc/pgx, goose, mise, docker-compose profiles, and add tests + GitHub Actions CI — without changing the HTTP API or handlers.

**Architecture:** Keep the `internal/data` model wrapper's public methods intact; swap their internals from `database/sql`+`lib/pq` to sqlc-generated code on a `pgxpool.Pool`. Migrations move to goose (same `migrations/` dir, also used as sqlc's schema source). Tooling moves from Makefile to `mise.toml`; local stacks defined in one `compose.yaml` with profiles + YAML anchors.

**Tech Stack:** Go 1.22+, pgx/v5, sqlc, goose, mise, docker compose, mkcert, GitHub Actions, chi router (unchanged).

## Global Constraints

- Module path: `github.com/vancanhuit/greenlight` (verbatim).
- Go floor: `go 1.22` in `go.mod`.
- Vendoring is used: run `go mod vendor` after every dependency change; CI checks it is consistent.
- Each task = one branch → PR → CI → squash-merge → delete branch. Branch from `main`.
- `main` must compile after every merged PR.
- Handlers under `cmd/api/` and the HTTP routing must not change behaviour.
- sqlc config: engine `postgresql`, `sql_package: "pgx/v5"`, queries `internal/data/queries/`, schema `migrations/`, output package `db` at `internal/data/sqlc`.
- Error sentinels stay in `internal/data/models.go`: `ErrRecordNotFound`, `ErrEditConflict`, `ErrDuplicateEmail`.
- Unique-violation detection uses `pgconn.PgError.Code == "23505"` (not string matching).

---

### Task 1: Tooling — mise, compose, README

**Files:**
- Create: `mise.toml`
- Create: `compose.yaml`
- Create: `.env.example`
- Modify: `.gitignore` (add `.env`, `.certs/`)
- Delete: `Makefile`
- Modify: `README.md`

**Interfaces:**
- Produces: mise tasks `run`, `build`, `test`, `test:db`, `lint`, `sqlc`, `migrate:up`, `migrate:down`, `migrate:new`, `db:up`, `db:down`, `dev:up`, `dev:up:https`, `certs:setup`. Env var `GREENLIGHT_DB_DSN`. Compose profiles `test`, `dev`, `dev-https`. These names are consumed by Tasks 2, 3, 4, 10.

- [ ] **Step 1: Create `mise.toml`**

```toml
[tools]
go = "1.22"
sqlc = "1.27.0"
goose = "3.21.1"
golangci-lint = "1.61.0"

[env]
_.file = ".env"
GREENLIGHT_DB_DSN = "postgres://dev:dev@localhost:5432/greenlight?sslmode=disable"
GREENLIGHT_TEST_DB_DSN = "postgres://dev:dev@localhost:5433/greenlight_test?sslmode=disable"

[tasks.run]
description = "Run the API"
run = "go run ./cmd/api -db-dsn=$GREENLIGHT_DB_DSN"

[tasks.build]
description = "Build the API binary"
run = "go build -ldflags='-s' -o=./bin/api ./cmd/api"

[tasks.test]
description = "Run unit tests (no DB)"
run = "go test -race ./..."

[tasks."test:db"]
description = "Run tests including DB integration tests"
run = "GREENLIGHT_TEST_DB_DSN=$GREENLIGHT_TEST_DB_DSN go test -race ./..."

[tasks.lint]
description = "Run linters"
run = ["gofmt -l -d .", "go vet ./...", "golangci-lint run"]

[tasks.sqlc]
description = "Generate sqlc code"
run = "sqlc generate"

[tasks."migrate:up"]
description = "Apply all up migrations"
run = "goose -dir=./migrations postgres \"$GREENLIGHT_DB_DSN\" up"

[tasks."migrate:down"]
description = "Roll back the last migration"
run = "goose -dir=./migrations postgres \"$GREENLIGHT_DB_DSN\" down"

[tasks."migrate:new"]
description = "Create a new migration: mise run migrate:new name=create_x"
run = "goose -dir=./migrations create ${name} sql"

[tasks."db:up"]
description = "Start the test Postgres stack"
run = "docker compose --profile test up -d"

[tasks."db:down"]
description = "Stop the test Postgres stack"
run = "docker compose --profile test down"

[tasks."dev:up"]
description = "Start dev stack (plain HTTP)"
run = "docker compose --profile dev up -d"

[tasks."dev:up:https"]
description = "Start dev stack (HTTPS via mkcert + caddy)"
run = "docker compose --profile dev-https up -d"

[tasks."certs:setup"]
description = "Generate local TLS certs with mkcert"
run = ["mkdir -p .certs", "mkcert -install", "mkcert -cert-file .certs/localhost.pem -key-file .certs/localhost-key.pem localhost 127.0.0.1"]
```

- [ ] **Step 2: Create `compose.yaml` with anchors + profiles**

```yaml
x-postgres-base: &pg-base
  image: postgres:16
  restart: unless-stopped
  environment:
    POSTGRES_USER: dev
    POSTGRES_PASSWORD: dev
  healthcheck:
    test: ["CMD-SHELL", "pg_isready -U dev"]
    interval: 10s
    timeout: 5s
    retries: 5

x-api-base: &api-base
  build:
    context: .
    dockerfile: Dockerfile
  environment:
    GREENLIGHT_DB_DSN: postgres://dev:dev@postgres-dev:5432/greenlight?sslmode=disable

services:
  postgres-dev:
    <<: *pg-base
    profiles: ["dev", "dev-https"]
    environment:
      POSTGRES_USER: dev
      POSTGRES_PASSWORD: dev
      POSTGRES_DB: greenlight
    ports:
      - "5432:5432"
    volumes:
      - pgdata:/var/lib/postgresql/data

  postgres-test:
    <<: *pg-base
    profiles: ["test"]
    environment:
      POSTGRES_USER: dev
      POSTGRES_PASSWORD: dev
      POSTGRES_DB: greenlight_test
    ports:
      - "5433:5432"
    tmpfs:
      - /var/lib/postgresql/data

  api:
    <<: *api-base
    profiles: ["dev"]
    command: ["-db-dsn=postgres://dev:dev@postgres-dev:5432/greenlight?sslmode=disable", "-port=8000"]
    ports:
      - "8000:8000"
    depends_on:
      postgres-dev:
        condition: service_healthy

  api-https:
    <<: *api-base
    profiles: ["dev-https"]
    command: ["-db-dsn=postgres://dev:dev@postgres-dev:5432/greenlight?sslmode=disable", "-port=8000"]
    depends_on:
      postgres-dev:
        condition: service_healthy

  caddy:
    image: caddy:2
    profiles: ["dev-https"]
    ports:
      - "443:443"
    volumes:
      - ./.certs:/certs:ro
      - ./Caddyfile:/etc/caddy/Caddyfile:ro
    depends_on:
      - api-https

volumes:
  pgdata:
```

- [ ] **Step 3: Create `Caddyfile` (used by dev-https profile)**

```
localhost {
    tls /certs/localhost.pem /certs/localhost-key.pem
    reverse_proxy api-https:8000
}
```

- [ ] **Step 4: Create `.env.example`**

```bash
GREENLIGHT_DB_DSN=postgres://dev:dev@localhost:5432/greenlight?sslmode=disable
GREENLIGHT_TEST_DB_DSN=postgres://dev:dev@localhost:5433/greenlight_test?sslmode=disable
```

- [ ] **Step 5: Update `.gitignore`**

Append these lines:

```
.env
.certs/
```

- [ ] **Step 6: Delete `Makefile`**

Run: `git rm Makefile`

- [ ] **Step 7: Rewrite the "Local development" section of `README.md`**

Replace the Tools + commands block with:

````markdown
## Local development

Tools are managed by [mise](https://mise.jdx.dev/). Install mise, then:

```bash
mise install                 # install go, sqlc, goose, golangci-lint
cp .env.example .env
mise run db:up               # start test Postgres (docker compose profile: test)
mise run dev:up              # start dev stack over HTTP
mise run certs:setup         # one-time: local TLS certs via mkcert
mise run dev:up:https        # dev stack over HTTPS (caddy + mkcert)
mise run migrate:up          # apply migrations
mise run run                 # run the API locally
mise run test                # unit tests
mise run test:db             # unit + DB integration tests
```
````

- [ ] **Step 8: Verify tooling parses**

Run: `mise tasks`
Expected: lists `run`, `build`, `test`, `test:db`, `db:up`, `dev:up`, `dev:up:https`, `migrate:up`, etc. with no parse error.

Run: `docker compose --profile test config -q`
Expected: exits 0 (valid compose file).

- [ ] **Step 9: Commit**

```bash
git add mise.toml compose.yaml Caddyfile .env.example .gitignore README.md
git rm Makefile
git commit -m "chore: replace Makefile with mise + compose profiles"
```

---

### Task 2: GitHub Actions CI skeleton

**Files:**
- Create: `.github/workflows/ci.yml`

**Interfaces:**
- Consumes: mise tasks `lint` from Task 1.
- Produces: workflow jobs `lint`, `audit` (the `test` job is stubbed here and wired to Postgres in Task 10).

- [ ] **Step 1: Create `.github/workflows/ci.yml`**

```yaml
name: CI

on:
  push:
    branches: [main]
  pull_request:

permissions:
  contents: read

jobs:
  lint:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: jdx/mise-action@v2
      - run: gofmt -l -d .
      - run: go vet ./...
      - run: golangci-lint run

  audit:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: jdx/mise-action@v2
      - run: go mod verify
      - run: go mod vendor
      - run: git diff --exit-code
```

- [ ] **Step 2: Validate YAML locally**

Run: `go run gopkg.in/yaml.v3 2>/dev/null; python3 -c "import yaml,sys; yaml.safe_load(open('.github/workflows/ci.yml'))" && echo OK`
Expected: `OK` (valid YAML).

- [ ] **Step 3: Commit**

```bash
git add .github/workflows/ci.yml
git commit -m "ci: add lint and audit workflows"
```

---

### Task 3: Convert migrations to goose

**Files:**
- Delete: all `migrations/0000*_*.up.sql` and `*.down.sql` (10 files)
- Create: `migrations/00001_create_movies_table.sql`
- Create: `migrations/00002_add_movies_indexes.sql`
- Create: `migrations/00003_create_users_table.sql`
- Create: `migrations/00004_create_tokens_table.sql`
- Create: `migrations/00005_add_permissions.sql`

**Interfaces:**
- Produces: goose-annotated migrations that double as sqlc's schema source (consumed by Task 4).

- [ ] **Step 1: Remove old golang-migrate files**

Run: `git rm migrations/0000*.up.sql migrations/0000*.down.sql`

- [ ] **Step 2: Create `migrations/00001_create_movies_table.sql`**

```sql
-- +goose Up
CREATE TABLE IF NOT EXISTS movies (
    id bigserial PRIMARY KEY,
    created_at timestamp(0) with time zone NOT NULL DEFAULT NOW(),
    title text NOT NULL,
    year integer NOT NULL,
    runtime integer NOT NULL,
    genres text[] NOT NULL,
    version integer NOT NULL DEFAULT 1
);

ALTER TABLE movies ADD CONSTRAINT movies_runtime_check CHECK (runtime >= 0);
ALTER TABLE movies ADD CONSTRAINT movies_year_check CHECK (year BETWEEN 1888 AND date_part('year', now()));
ALTER TABLE movies ADD CONSTRAINT genres_length_check CHECK (array_length(genres, 1) BETWEEN 1 AND 5);

-- +goose Down
DROP TABLE IF EXISTS movies;
```

- [ ] **Step 3: Create `migrations/00002_add_movies_indexes.sql`**

```sql
-- +goose Up
CREATE INDEX IF NOT EXISTS movies_title_idx ON movies USING GIN (to_tsvector('simple', title));
CREATE INDEX IF NOT EXISTS movies_genres_idx ON movies USING GIN (genres);

-- +goose Down
DROP INDEX IF EXISTS movies_title_idx;
DROP INDEX IF EXISTS movies_genres_idx;
```

- [ ] **Step 4: Create `migrations/00003_create_users_table.sql`**

```sql
-- +goose Up
CREATE EXTENSION IF NOT EXISTS citext;

CREATE TABLE IF NOT EXISTS users (
    id bigserial PRIMARY KEY,
    created_at timestamp(0) with time zone NOT NULL DEFAULT NOW(),
    name text NOT NULL,
    email citext UNIQUE NOT NULL,
    password_hash bytea NOT NULL,
    activated bool NOT NULL,
    version integer NOT NULL DEFAULT 1
);

-- +goose Down
DROP TABLE IF EXISTS users;
DROP EXTENSION IF EXISTS citext;
```

- [ ] **Step 5: Create `migrations/00004_create_tokens_table.sql`**

```sql
-- +goose Up
CREATE TABLE IF NOT EXISTS tokens (
    hash bytea PRIMARY KEY,
    user_id bigint NOT NULL REFERENCES users ON DELETE CASCADE,
    expiry timestamp(0) with time zone NOT NULL,
    scope text NOT NULL
);

-- +goose Down
DROP TABLE IF EXISTS tokens;
```

- [ ] **Step 6: Create `migrations/00005_add_permissions.sql`**

```sql
-- +goose Up
CREATE TABLE IF NOT EXISTS permissions (
    id bigserial PRIMARY KEY,
    code text NOT NULL
);

CREATE TABLE IF NOT EXISTS users_permissions (
    user_id bigint NOT NULL REFERENCES users ON DELETE CASCADE,
    permission_id bigint NOT NULL REFERENCES permissions ON DELETE CASCADE,
    PRIMARY KEY (user_id, permission_id)
);

INSERT INTO permissions (code) VALUES ('movies:read'), ('movies:write');

-- +goose Down
DROP TABLE IF EXISTS users_permissions;
DROP TABLE IF EXISTS permissions;
```

- [ ] **Step 7: Verify goose applies cleanly against the test DB**

Run: `mise run db:up && goose -dir=./migrations postgres "$GREENLIGHT_TEST_DB_DSN" up && goose -dir=./migrations postgres "$GREENLIGHT_TEST_DB_DSN" status`
Expected: all 5 migrations show as applied, no errors.

- [ ] **Step 8: Commit**

```bash
git add migrations/
git commit -m "feat: convert migrations to goose format"
```

---

### Task 4: sqlc + pgx wiring and the movies model

**Files:**
- Create: `sqlc.yaml`
- Create: `internal/data/queries/movies.sql`
- Create (generated): `internal/data/sqlc/*` via `sqlc generate`
- Modify: `go.mod` / `go.sum` (add `github.com/jackc/pgx/v5`)
- Modify: `cmd/api/main.go` (openDB → pgxpool)
- Modify: `internal/data/models.go` (`NewModels(pool)`, hold `*db.Queries`)
- Modify: `internal/data/movies.go` (methods delegate to sqlc + pgxpool)

**Interfaces:**
- Consumes: goose schema from Task 3.
- Produces:
  - `db.New(pool) *db.Queries` (sqlc generated).
  - `data.NewModels(pool *pgxpool.Pool) Models`.
  - `MovieModel` with unchanged public methods: `Insert(*Movie) error`, `Get(int64) (*Movie, error)`, `GetAll(string, []string, Filters) ([]*Movie, Metadata, error)`, `Update(*Movie) error`, `Delete(int64) error`.

- [ ] **Step 1: Add pgx dependency**

Run: `go get github.com/jackc/pgx/v5@latest && go mod edit -go=1.22`
Expected: `go.mod` now requires `github.com/jackc/pgx/v5`.

- [ ] **Step 2: Create `sqlc.yaml`**

```yaml
version: "2"
sql:
  - engine: "postgresql"
    schema: "migrations"
    queries: "internal/data/queries"
    gen:
      go:
        package: "db"
        out: "internal/data/sqlc"
        sql_package: "pgx/v5"
        emit_json_tags: false
        emit_pointers_for_null_types: true
```

- [ ] **Step 3: Create `internal/data/queries/movies.sql`**

Note: `GetAll` needs dynamic `ORDER BY`, which sqlc cannot parameterize, so it is NOT defined here — it stays a raw `pgxpool` query in `movies.go` (Step 7). Only fixed-shape queries go through sqlc.

```sql
-- name: InsertMovie :one
INSERT INTO movies (title, year, runtime, genres)
VALUES ($1, $2, $3, $4)
RETURNING id, created_at, version;

-- name: GetMovie :one
SELECT id, created_at, title, year, runtime, genres, version
FROM movies
WHERE id = $1;

-- name: UpdateMovie :one
UPDATE movies
SET title = $1, year = $2, runtime = $3, genres = $4, version = version + 1
WHERE id = $5 AND version = $6
RETURNING version;

-- name: DeleteMovie :execrows
DELETE FROM movies WHERE id = $1;
```

- [ ] **Step 4: Generate sqlc code**

Run: `mise run sqlc`
Expected: creates `internal/data/sqlc/db.go`, `models.go`, `movies.sql.go` with a `Queries` type and methods `InsertMovie`, `GetMovie`, `UpdateMovie`, `DeleteMovie`. No errors.

- [ ] **Step 5: Convert `openDB` in `cmd/api/main.go` to pgxpool**

Replace the `database/sql` import block and `openDB` function. Change import `_ "github.com/lib/pq"` and `"database/sql"` to:

```go
	"github.com/jackc/pgx/v5/pgxpool"
```

Replace `openDB`:

```go
func openDB(cfg config) (*pgxpool.Pool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	poolCfg, err := pgxpool.ParseConfig(cfg.db.dsn)
	if err != nil {
		return nil, err
	}

	poolCfg.MaxConns = int32(cfg.db.maxOpenConns)

	duration, err := time.ParseDuration(cfg.db.maxIdleTime)
	if err != nil {
		return nil, err
	}
	poolCfg.MaxConnIdleTime = duration

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, err
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}

	return pool, nil
}
```

In `main()`, change `defer db.Close()` (unchanged — `*pgxpool.Pool` has `Close()`), and change the `expvar.Publish("database", ...)` block to remove `db.Stats()` (pgxpool exposes `pool.Stat()`):

```go
	expvar.Publish("database", expvar.Func(func() any {
		return db.Stat()
	}))
```

- [ ] **Step 6: Update `internal/data/models.go`**

```go
package data

import (
	"errors"

	"github.com/jackc/pgx/v5/pgxpool"
	db "github.com/vancanhuit/greenlight/internal/data/sqlc"
)

type Models struct {
	Movies      MovieModel
	Users       UserModel
	Tokens      TokenModel
	Permissions PermissionModel
}

var (
	ErrRecordNotFound = errors.New("record not found")
	ErrEditConflict   = errors.New("edit conflict")
	ErrDuplicateEmail = errors.New("duplicate email")
)

func NewModels(pool *pgxpool.Pool) Models {
	q := db.New(pool)
	return Models{
		Movies:      MovieModel{q: q, pool: pool},
		Users:       UserModel{q: q},
		Tokens:      TokenModel{q: q},
		Permissions: PermissionModel{q: q},
	}
}
```

- [ ] **Step 7: Rewrite `internal/data/movies.go` method bodies**

Keep `Movie`, `ValidateMovie` unchanged. Replace `MovieModel` struct and all methods:

```go
package data

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	db "github.com/vancanhuit/greenlight/internal/data/sqlc"
)

type MovieModel struct {
	q    *db.Queries
	pool *pgxpool.Pool
}

func (m MovieModel) Insert(movie *Movie) error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	row, err := m.q.InsertMovie(ctx, db.InsertMovieParams{
		Title:   movie.Title,
		Year:    movie.Year,
		Runtime: int32(movie.Runtime),
		Genres:  movie.Genres,
	})
	if err != nil {
		return err
	}
	movie.ID = row.ID
	movie.CreatedAt = row.CreatedAt
	movie.Version = row.Version
	return nil
}

func (m MovieModel) Get(id int64) (*Movie, error) {
	if id < 1 {
		return nil, ErrRecordNotFound
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	row, err := m.q.GetMovie(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrRecordNotFound
		}
		return nil, err
	}

	return &Movie{
		ID:        row.ID,
		CreatedAt: row.CreatedAt,
		Title:     row.Title,
		Year:      row.Year,
		Runtime:   Runtime(row.Runtime),
		Genres:    row.Genres,
		Version:   row.Version,
	}, nil
}

func (m MovieModel) GetAll(title string, genres []string, filters Filters) ([]*Movie, Metadata, error) {
	query := fmt.Sprintf(`SELECT count(*) OVER(), id, created_at, title, year, runtime, genres, version
	FROM movies
	WHERE (to_tsvector('simple', title) @@ plainto_tsquery('simple', $1) OR $1 = '') AND (genres @> $2 OR $2 = '{}')
	ORDER BY %s %s, id
	LIMIT $3 OFFSET $4`, filters.sortColumn(), filters.sortDirection())

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	rows, err := m.pool.Query(ctx, query, title, genres, filters.limit(), filters.offset())
	if err != nil {
		return nil, Metadata{}, err
	}
	defer rows.Close()

	movies := []*Movie{}
	totalRecords := 0
	for rows.Next() {
		var movie Movie
		var runtime int32
		err := rows.Scan(
			&totalRecords,
			&movie.ID,
			&movie.CreatedAt,
			&movie.Title,
			&movie.Year,
			&runtime,
			&movie.Genres,
			&movie.Version,
		)
		if err != nil {
			return nil, Metadata{}, err
		}
		movie.Runtime = Runtime(runtime)
		movies = append(movies, &movie)
	}
	if err := rows.Err(); err != nil {
		return nil, Metadata{}, err
	}

	metadata := caculateMetadata(totalRecords, filters.Page, filters.PageSize)
	return movies, metadata, nil
}

func (m MovieModel) Update(movie *Movie) error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	version, err := m.q.UpdateMovie(ctx, db.UpdateMovieParams{
		Title:   movie.Title,
		Year:    movie.Year,
		Runtime: int32(movie.Runtime),
		Genres:  movie.Genres,
		ID:      movie.ID,
		Version: movie.Version,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrEditConflict
		}
		return err
	}
	movie.Version = version
	return nil
}

func (m MovieModel) Delete(id int64) error {
	if id < 1 {
		return ErrRecordNotFound
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	rows, err := m.q.DeleteMovie(ctx, id)
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrRecordNotFound
	}
	return nil
}
```

Note: `Movie.Runtime` is the `Runtime` type (int32 underlying). sqlc maps `integer` → `int32`; convert both ways. `Movie.Year`/`Version` are already `int32`, matching sqlc.

- [ ] **Step 8: Build**

Run: `go build ./...`
Expected: compiles. (`users.go`, `tokens.go`, `permissions.go` still use the old `DB *sql.DB` field — they are ported in Tasks 5-7. To keep `main` green, this task ports movies only; Tasks 5-7 follow before removing `lib/pq` in Task 8. If the build breaks because `NewModels` no longer passes `*sql.DB`, port the other three structs' field type to `q *db.Queries` in the SAME PR with temporary method bodies that still work — see note below.)

**Important sequencing note:** Because `NewModels` changes signature, all four models must at least *compile* in this PR. To keep tasks reviewable, this PR (Task 4) updates the three not-yet-ported structs (`UserModel`, `TokenModel`, `PermissionModel`) to hold `q *db.Queries` and temporarily keep their SQL by using `m.q` is not possible for raw SQL. Instead, keep them on the pool: give each a `pool *pgxpool.Pool` field too and leave their existing raw-SQL bodies running through `pool.Query`/`pool.Exec` until Tasks 5-7 convert them to sqlc queries. Simplest: in this PR, port all four to pgxpool raw queries (mechanical `database/sql`→`pgxpool` swap), then Tasks 5-7 replace raw SQL with sqlc query calls one model at a time. Update the plan executor: **do the raw pgxpool swap for users/tokens/permissions here so the build is green, then Tasks 5-7 move each to sqlc.**

- [ ] **Step 9: Smoke test against DB**

Run: `mise run db:up && goose -dir=./migrations postgres "$GREENLIGHT_TEST_DB_DSN" up && GREENLIGHT_DB_DSN="$GREENLIGHT_TEST_DB_DSN" go run ./cmd/api -db-dsn="$GREENLIGHT_TEST_DB_DSN" -port=8001 &` then `curl -s localhost:8001/v1/healthcheck` then kill the server.
Expected: healthcheck JSON returned; server logs "database connection pool established".

- [ ] **Step 10: Vendor + commit**

```bash
go mod tidy && go mod vendor
git add sqlc.yaml go.mod go.sum vendor/ internal/data/ cmd/api/main.go
git commit -m "feat: wire sqlc+pgx and port movies model"
```

---

### Task 5: Port users model to sqlc

**Files:**
- Create: `internal/data/queries/users.sql`
- Modify: `internal/data/users.go`
- Regenerate: `internal/data/sqlc/*`

**Interfaces:**
- Consumes: `db.Queries`, `pgconn.PgError` for unique-violation detection.
- Produces: `UserModel` unchanged public methods: `Insert(*User) error`, `GetByEmail(string) (*User, error)`, `Update(*User) error`, `GetForToken(string, string) (*User, error)`.

- [ ] **Step 1: Create `internal/data/queries/users.sql`**

```sql
-- name: InsertUser :one
INSERT INTO users (name, email, password_hash, activated)
VALUES ($1, $2, $3, $4)
RETURNING id, created_at, version;

-- name: GetUserByEmail :one
SELECT id, created_at, name, email, password_hash, activated, version
FROM users
WHERE email = $1;

-- name: UpdateUser :one
UPDATE users
SET name = $1, email = $2, password_hash = $3, activated = $4, version = version + 1
WHERE id = $5 AND version = $6
RETURNING version;

-- name: GetUserForToken :one
SELECT users.id, users.created_at, users.name, users.email, users.password_hash, users.activated, users.version
FROM users
INNER JOIN tokens ON users.id = tokens.user_id
WHERE tokens.hash = $1 AND tokens.scope = $2 AND tokens.expiry > $3;
```

- [ ] **Step 2: Regenerate**

Run: `mise run sqlc`
Expected: new methods `InsertUser`, `GetUserByEmail`, `UpdateUser`, `GetUserForToken` on `Queries`.

- [ ] **Step 3: Add a unique-violation helper in `internal/data/models.go`**

Append:

```go
func isUniqueViolation(err error, constraint string) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23505" && pgErr.ConstraintName == constraint
	}
	return false
}
```

Add import `"github.com/jackc/pgx/v5/pgconn"`.

- [ ] **Step 4: Rewrite `internal/data/users.go` DB methods**

Keep `password`, `User`, `AnonymousUser`, all `Validate*` funcs unchanged. Replace `UserModel` struct + `Insert`/`GetByEmail`/`Update`/`GetForToken`:

```go
type UserModel struct {
	q *db.Queries
}

func (m UserModel) Insert(user *User) error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	row, err := m.q.InsertUser(ctx, db.InsertUserParams{
		Name:         user.Name,
		Email:        user.Email,
		PasswordHash: user.Password.hash,
		Activated:    user.Activated,
	})
	if err != nil {
		if isUniqueViolation(err, "users_email_key") {
			return ErrDuplicateEmail
		}
		return err
	}
	user.ID = row.ID
	user.CreatedAt = row.CreatedAt
	user.Version = int(row.Version)
	return nil
}

func (m UserModel) GetByEmail(email string) (*User, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	row, err := m.q.GetUserByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrRecordNotFound
		}
		return nil, err
	}
	return userFromRow(row.ID, row.CreatedAt, row.Name, row.Email, row.PasswordHash, row.Activated, row.Version), nil
}

func (m UserModel) Update(user *User) error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	version, err := m.q.UpdateUser(ctx, db.UpdateUserParams{
		Name:         user.Name,
		Email:        user.Email,
		PasswordHash: user.Password.hash,
		Activated:    user.Activated,
		ID:           user.ID,
		Version:      int32(user.Version),
	})
	if err != nil {
		switch {
		case isUniqueViolation(err, "users_email_key"):
			return ErrDuplicateEmail
		case errors.Is(err, pgx.ErrNoRows):
			return ErrEditConflict
		default:
			return err
		}
	}
	user.Version = int(version)
	return nil
}

func (m UserModel) GetForToken(tokenScope string, tokenPlaintext string) (*User, error) {
	tokenHash := sha256.Sum256([]byte(tokenPlaintext))

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	row, err := m.q.GetUserForToken(ctx, db.GetUserForTokenParams{
		Hash:   tokenHash[:],
		Scope:  tokenScope,
		Expiry: time.Now(),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrRecordNotFound
		}
		return nil, err
	}
	return userFromRow(row.ID, row.CreatedAt, row.Name, row.Email, row.PasswordHash, row.Activated, row.Version), nil
}

func userFromRow(id int64, createdAt time.Time, name, email string, passwordHash []byte, activated bool, version int32) *User {
	u := &User{
		ID:        id,
		CreatedAt: createdAt,
		Name:      name,
		Email:     email,
		Activated: activated,
		Version:   int(version),
	}
	u.Password.hash = passwordHash
	return u
}
```

Update imports: drop `"database/sql"`; add `"github.com/jackc/pgx/v5"` and `db "github.com/vancanhuit/greenlight/internal/data/sqlc"`.

Note: the `GetUserForToken` param for `expiry` is `timestamptz`; sqlc types it `time.Time`. The `hash` param is `bytea` → `[]byte`.

- [ ] **Step 5: Build + vendor**

Run: `go build ./... && go mod vendor`
Expected: compiles clean.

- [ ] **Step 6: Commit**

```bash
git add internal/data/ vendor/
git commit -m "feat: port users model to sqlc"
```

---

### Task 6: Port tokens model to sqlc

**Files:**
- Create: `internal/data/queries/tokens.sql`
- Modify: `internal/data/tokens.go`
- Regenerate: `internal/data/sqlc/*`

**Interfaces:**
- Produces: `TokenModel` unchanged public methods `New`, `Insert(*Token) error`, `DeleteAllForUser(string, int64) error`.

- [ ] **Step 1: Create `internal/data/queries/tokens.sql`**

```sql
-- name: InsertToken :exec
INSERT INTO tokens (hash, user_id, expiry, scope)
VALUES ($1, $2, $3, $4);

-- name: DeleteAllTokensForUser :exec
DELETE FROM tokens WHERE scope = $1 AND user_id = $2;
```

- [ ] **Step 2: Regenerate**

Run: `mise run sqlc`
Expected: `InsertToken`, `DeleteAllTokensForUser` methods added.

- [ ] **Step 3: Rewrite `internal/data/tokens.go` DB methods**

Keep `Token`, `generateToken`, `ValidateTokenPlaintext`, `New` unchanged. Replace `TokenModel` struct + `Insert`/`DeleteAllForUser`:

```go
type TokenModel struct {
	q *db.Queries
}

func (m TokenModel) Insert(token *Token) error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	return m.q.InsertToken(ctx, db.InsertTokenParams{
		Hash:   token.Hash,
		UserID: token.UserID,
		Expiry: token.Expiry,
		Scope:  token.Scope,
	})
}

func (m TokenModel) DeleteAllForUser(scope string, userID int64) error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	return m.q.DeleteAllTokensForUser(ctx, db.DeleteAllTokensForUserParams{
		Scope:  scope,
		UserID: userID,
	})
}
```

Update imports: drop `"database/sql"`; add `db "github.com/vancanhuit/greenlight/internal/data/sqlc"`.

- [ ] **Step 4: Build + vendor**

Run: `go build ./... && go mod vendor`
Expected: compiles clean.

- [ ] **Step 5: Commit**

```bash
git add internal/data/ vendor/
git commit -m "feat: port tokens model to sqlc"
```

---

### Task 7: Port permissions model to sqlc

**Files:**
- Create: `internal/data/queries/permissions.sql`
- Modify: `internal/data/permissions.go`
- Regenerate: `internal/data/sqlc/*`

**Interfaces:**
- Produces: `PermissionModel` unchanged public methods `GetAllForUser(int64) (Permissions, error)`, `AddForUser(int64, ...string) error`.

- [ ] **Step 1: Create `internal/data/queries/permissions.sql`**

```sql
-- name: GetPermissionsForUser :many
SELECT permissions.code
FROM permissions
INNER JOIN users_permissions ON users_permissions.permission_id = permissions.id
INNER JOIN users ON users_permissions.user_id = users.id
WHERE users.id = $1;

-- name: AddPermissionsForUser :exec
INSERT INTO users_permissions
SELECT $1, permissions.id FROM permissions WHERE permissions.code = ANY($2::text[]);
```

- [ ] **Step 2: Regenerate**

Run: `mise run sqlc`
Expected: `GetPermissionsForUser` (returns `[]string`) and `AddPermissionsForUser` methods. The `ANY($2::text[])` param maps to `[]string`.

- [ ] **Step 3: Rewrite `internal/data/permissions.go`**

Keep `Permissions` type + `Include`. Replace struct + methods:

```go
package data

import (
	"context"
	"time"

	db "github.com/vancanhuit/greenlight/internal/data/sqlc"
)

type PermissionModel struct {
	q *db.Queries
}

func (m PermissionModel) GetAllForUser(userID int64) (Permissions, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	codes, err := m.q.GetPermissionsForUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	return Permissions(codes), nil
}

func (m PermissionModel) AddForUser(userID int64, codes ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	return m.q.AddPermissionsForUser(ctx, db.AddPermissionsForUserParams{
		UserID:  userID,
		Column2: codes,
	})
}
```

Note: verify the generated param field name for the `ANY($2::text[])` argument — sqlc may name it `Column2` or similar. Adjust the struct literal field to match the generated `AddPermissionsForUserParams`.

- [ ] **Step 4: Build + vendor**

Run: `go build ./... && go mod vendor`
Expected: compiles clean. If the param field name differs, fix it and rebuild.

- [ ] **Step 5: Commit**

```bash
git add internal/data/ vendor/
git commit -m "feat: port permissions model to sqlc"
```

---

### Task 8: Remove lib/pq and database/sql, re-vendor

**Files:**
- Modify: `go.mod`, `go.sum`
- Modify: `vendor/modules.txt` (via `go mod vendor`)
- Grep-verify: no remaining `lib/pq` / `database/sql` imports in app code

**Interfaces:**
- Consumes: all models ported (Tasks 4-7).
- Produces: dependency-clean module with only pgx as the driver.

- [ ] **Step 1: Confirm no residual imports**

Run: `grep -rn "lib/pq\|database/sql" --include=*.go cmd internal | grep -v _test.go`
Expected: no output.

- [ ] **Step 2: Remove the dependency + re-vendor**

Run: `go mod tidy && go mod vendor`
Expected: `github.com/lib/pq` removed from `go.mod`; `vendor/github.com/lib/pq` gone.

- [ ] **Step 3: Build + verify vendor consistency**

Run: `go build ./... && go mod verify && git diff --exit-code -- vendor/ || true`
Expected: builds clean; `go mod verify` prints `all modules verified`.

- [ ] **Step 4: Commit**

```bash
git add go.mod go.sum vendor/
git commit -m "chore: drop lib/pq and database/sql"
```

---

### Task 9: Unit tests (no DB)

**Files:**
- Create: `internal/validator/validator_test.go`
- Create: `internal/data/runtime_test.go`
- Create: `internal/data/filters_test.go`
- Create: `internal/jsonlog/jsonlog_test.go`

**Interfaces:**
- Consumes: existing pure functions — `validator.Validator`, `data.Runtime.MarshalJSON`, `data.Filters.sortColumn/sortDirection/limit/offset`, `jsonlog.Logger`.
- Produces: unit test coverage runnable with `go test ./...` and no database.

- [ ] **Step 1: Write `internal/validator/validator_test.go`**

```go
package validator

import "testing"

func TestValidatorValid(t *testing.T) {
	v := New()
	if !v.Valid() {
		t.Fatal("expected new validator to be valid")
	}
	v.AddError("field", "boom")
	if v.Valid() {
		t.Fatal("expected validator to be invalid after AddError")
	}
}

func TestValidatorCheck(t *testing.T) {
	v := New()
	v.Check(false, "field", "must be provided")
	if got := v.Errors["field"]; got != "must be provided" {
		t.Fatalf("got %q, want %q", got, "must be provided")
	}
}

func TestPermittedValue(t *testing.T) {
	if !PermittedValue("a", "a", "b") {
		t.Fatal("expected a to be permitted")
	}
	if PermittedValue("z", "a", "b") {
		t.Fatal("expected z to be rejected")
	}
}

func TestUnique(t *testing.T) {
	if !Unique([]string{"a", "b"}) {
		t.Fatal("expected unique")
	}
	if Unique([]string{"a", "a"}) {
		t.Fatal("expected non-unique")
	}
}
```

Note: confirm the actual exported helper names in `internal/validator/validator.go` (`New`, `Valid`, `AddError`, `Check`, `PermittedValue`/`In`, `Unique`, `Matches`, `EmailRX`). Adjust the test to the real names before running.

- [ ] **Step 2: Run it, expect pass**

Run: `go test ./internal/validator/ -v`
Expected: PASS.

- [ ] **Step 3: Write `internal/data/runtime_test.go`**

```go
package data

import "testing"

func TestRuntimeMarshalJSON(t *testing.T) {
	r := Runtime(102)
	b, err := r.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != `"102 mins"` {
		t.Fatalf("got %s, want %q", b, `"102 mins"`)
	}
}

func TestRuntimeUnmarshalJSON(t *testing.T) {
	var r Runtime
	if err := r.UnmarshalJSON([]byte(`"102 mins"`)); err != nil {
		t.Fatal(err)
	}
	if r != 102 {
		t.Fatalf("got %d, want 102", r)
	}
	if err := r.UnmarshalJSON([]byte(`"102"`)); err != ErrInvalidRuntimeFormat {
		t.Fatalf("expected ErrInvalidRuntimeFormat, got %v", err)
	}
}
```

Note: confirm `ErrInvalidRuntimeFormat` and `UnmarshalJSON` exist in `internal/data/runtime.go`; adjust if the format string differs.

- [ ] **Step 4: Write `internal/data/filters_test.go`**

```go
package data

import "testing"

func TestFiltersLimitOffset(t *testing.T) {
	f := Filters{Page: 3, PageSize: 20}
	if f.limit() != 20 {
		t.Fatalf("limit = %d, want 20", f.limit())
	}
	if f.offset() != 40 {
		t.Fatalf("offset = %d, want 40", f.offset())
	}
}

func TestFiltersSort(t *testing.T) {
	f := Filters{Sort: "-year", SortSafelist: []string{"year", "-year"}}
	if f.sortColumn() != "year" {
		t.Fatalf("sortColumn = %q, want year", f.sortColumn())
	}
	if f.sortDirection() != "DESC" {
		t.Fatalf("sortDirection = %q, want DESC", f.sortDirection())
	}
}
```

Note: confirm `Filters` field names (`Page`, `PageSize`, `Sort`, `SortSafelist`) and unexported method names in `internal/data/filters.go`.

- [ ] **Step 5: Write `internal/jsonlog/jsonlog_test.go`**

```go
package jsonlog

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestLoggerPrintInfo(t *testing.T) {
	var buf bytes.Buffer
	logger := New(&buf, LevelInfo)
	logger.PrintInfo("hello", map[string]string{"k": "v"})

	var entry map[string]any
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("log output not JSON: %v", err)
	}
	if entry["level"] != "INFO" || entry["message"] != "hello" {
		t.Fatalf("unexpected entry: %v", entry)
	}
}

func TestLoggerBelowMinLevelSuppressed(t *testing.T) {
	var buf bytes.Buffer
	logger := New(&buf, LevelError)
	logger.PrintInfo("skip me", nil)
	if buf.Len() != 0 {
		t.Fatalf("expected no output, got %q", buf.String())
	}
}
```

Note: confirm `New`, `LevelInfo`, `LevelError`, `PrintInfo` signatures in `internal/jsonlog/jsonlog.go`.

- [ ] **Step 6: Run all unit tests**

Run: `go test -race ./...`
Expected: PASS for validator, data, jsonlog packages; other packages `[no test files]`.

- [ ] **Step 7: Commit**

```bash
git add internal/
git commit -m "test: add unit tests for validator, runtime, filters, jsonlog"
```

---

### Task 10: DB integration tests + CI test job

**Files:**
- Create: `internal/data/main_test.go` (TestMain, helpers)
- Create: `internal/data/movies_db_test.go`
- Create: `internal/data/users_db_test.go`
- Modify: `.github/workflows/ci.yml` (add `test` job with Postgres service)

**Interfaces:**
- Consumes: `NewModels(pool)`, goose migrations, `GREENLIGHT_TEST_DB_DSN` env var.
- Produces: DB tests skipped when `GREENLIGHT_TEST_DB_DSN` unset; CI runs them against a Postgres service container.

- [ ] **Step 1: Add goose as a library dep for programmatic migration in TestMain**

Run: `go get github.com/pressly/goose/v3@latest`
Expected: added to `go.mod`.

- [ ] **Step 2: Create `internal/data/main_test.go`**

```go
package data_test

import (
	"context"
	"database/sql"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

var testPool *pgxpool.Pool

func TestMain(m *testing.M) {
	dsn := os.Getenv("GREENLIGHT_TEST_DB_DSN")
	if dsn == "" {
		// No test DB configured: skip DB tests entirely.
		os.Exit(m.Run())
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		panic(err)
	}
	testPool = pool

	sqlDB := stdlib.OpenDBFromPool(pool)
	if err := goose.SetDialect("postgres"); err != nil {
		panic(err)
	}
	if err := goose.Up(sqlDB, "../../migrations"); err != nil {
		panic(err)
	}
	_ = sqlDB.Close()

	code := m.Run()
	pool.Close()
	os.Exit(code)
}

func truncate(t *testing.T, tables ...string) {
	t.Helper()
	if testPool == nil {
		return
	}
	for _, tbl := range tables {
		_, err := testPool.Exec(context.Background(), "TRUNCATE "+tbl+" RESTART IDENTITY CASCADE")
		if err != nil {
			t.Fatalf("truncate %s: %v", tbl, err)
		}
	}
}

func requireDB(t *testing.T) {
	t.Helper()
	if testPool == nil {
		t.Skip("GREENLIGHT_TEST_DB_DSN not set; skipping DB test")
	}
}

// silence unused import if stdlib version changes signature
var _ = sql.LevelDefault
```

Note: `stdlib.OpenDBFromPool` exists in pgx/v5 ≥ v5.4. If unavailable, open a second `*sql.DB` with `sql.Open("pgx", dsn)` using the stdlib driver import `_ "github.com/jackc/pgx/v5/stdlib"`. Remove the `var _ = sql.LevelDefault` line and the `database/sql` import if not needed.

- [ ] **Step 3: Create `internal/data/movies_db_test.go`**

```go
package data_test

import (
	"errors"
	"testing"

	"github.com/vancanhuit/greenlight/internal/data"
)

func TestMovieInsertGet(t *testing.T) {
	requireDB(t)
	truncate(t, "movies")

	models := data.NewModels(testPool)
	movie := &data.Movie{Title: "Casablanca", Year: 1942, Runtime: 102, Genres: []string{"drama"}}

	if err := models.Movies.Insert(movie); err != nil {
		t.Fatalf("insert: %v", err)
	}
	if movie.ID == 0 {
		t.Fatal("expected generated ID")
	}

	got, err := models.Movies.Get(movie.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Title != "Casablanca" || got.Year != 1942 || got.Runtime != 102 {
		t.Fatalf("unexpected movie: %+v", got)
	}
}

func TestMovieGetNotFound(t *testing.T) {
	requireDB(t)
	truncate(t, "movies")

	models := data.NewModels(testPool)
	_, err := models.Movies.Get(999)
	if !errors.Is(err, data.ErrRecordNotFound) {
		t.Fatalf("expected ErrRecordNotFound, got %v", err)
	}
}

func TestMovieUpdateEditConflict(t *testing.T) {
	requireDB(t)
	truncate(t, "movies")

	models := data.NewModels(testPool)
	movie := &data.Movie{Title: "Alien", Year: 1979, Runtime: 117, Genres: []string{"sci-fi"}}
	if err := models.Movies.Insert(movie); err != nil {
		t.Fatalf("insert: %v", err)
	}

	stale := *movie
	stale.Version = movie.Version + 1 // wrong version
	stale.Title = "Aliens"
	if err := models.Movies.Update(&stale); !errors.Is(err, data.ErrEditConflict) {
		t.Fatalf("expected ErrEditConflict, got %v", err)
	}
}

func TestMovieDelete(t *testing.T) {
	requireDB(t)
	truncate(t, "movies")

	models := data.NewModels(testPool)
	movie := &data.Movie{Title: "Heat", Year: 1995, Runtime: 170, Genres: []string{"crime"}}
	if err := models.Movies.Insert(movie); err != nil {
		t.Fatalf("insert: %v", err)
	}
	if err := models.Movies.Delete(movie.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if err := models.Movies.Delete(movie.ID); !errors.Is(err, data.ErrRecordNotFound) {
		t.Fatalf("expected ErrRecordNotFound on second delete, got %v", err)
	}
}
```

- [ ] **Step 4: Create `internal/data/users_db_test.go`**

```go
package data_test

import (
	"errors"
	"testing"

	"github.com/vancanhuit/greenlight/internal/data"
)

func newTestUser(t *testing.T, email string) *data.User {
	t.Helper()
	u := &data.User{Name: "Alice", Email: email, Activated: true}
	if err := u.Password.Set("password123"); err != nil {
		t.Fatalf("set password: %v", err)
	}
	return u
}

func TestUserInsertDuplicateEmail(t *testing.T) {
	requireDB(t)
	truncate(t, "users")

	models := data.NewModels(testPool)
	if err := models.Users.Insert(newTestUser(t, "a@example.com")); err != nil {
		t.Fatalf("first insert: %v", err)
	}
	err := models.Users.Insert(newTestUser(t, "a@example.com"))
	if !errors.Is(err, data.ErrDuplicateEmail) {
		t.Fatalf("expected ErrDuplicateEmail, got %v", err)
	}
}

func TestUserGetByEmail(t *testing.T) {
	requireDB(t)
	truncate(t, "users")

	models := data.NewModels(testPool)
	u := newTestUser(t, "b@example.com")
	if err := models.Users.Insert(u); err != nil {
		t.Fatalf("insert: %v", err)
	}

	got, err := models.Users.GetByEmail("b@example.com")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Name != "Alice" || !got.Activated {
		t.Fatalf("unexpected user: %+v", got)
	}
}
```

Note: `data.User.Password.Set` is exported behaviour via the `password.Set` method — confirm `Password` field is addressable (`u.Password.Set(...)`); it is, since `Set` has a pointer receiver and `u` is a pointer.

- [ ] **Step 5: Run DB tests locally**

Run: `mise run db:up && GREENLIGHT_TEST_DB_DSN="$GREENLIGHT_TEST_DB_DSN" go test -race ./internal/data/ -run 'Movie|User' -v`
Expected: all DB tests PASS. Then `go test ./...` (without the env var) → same DB tests SKIP.

- [ ] **Step 6: Add the `test` job to `.github/workflows/ci.yml`**

Append under `jobs:`:

```yaml
  test:
    runs-on: ubuntu-latest
    services:
      postgres:
        image: postgres:16
        env:
          POSTGRES_USER: dev
          POSTGRES_PASSWORD: dev
          POSTGRES_DB: greenlight_test
        ports:
          - 5433:5432
        options: >-
          --health-cmd "pg_isready -U dev"
          --health-interval 10s
          --health-timeout 5s
          --health-retries 5
    env:
      GREENLIGHT_TEST_DB_DSN: postgres://dev:dev@localhost:5433/greenlight_test?sslmode=disable
    steps:
      - uses: actions/checkout@v4
      - uses: jdx/mise-action@v2
      - run: goose -dir=./migrations postgres "$GREENLIGHT_TEST_DB_DSN" up
      - run: go test -race ./...
```

- [ ] **Step 7: Validate workflow YAML**

Run: `python3 -c "import yaml; yaml.safe_load(open('.github/workflows/ci.yml'))" && echo OK`
Expected: `OK`.

- [ ] **Step 8: Vendor + commit**

```bash
go mod tidy && go mod vendor
git add internal/data/ .github/workflows/ci.yml go.mod go.sum vendor/
git commit -m "test: add DB integration tests and CI test job"
```

---

## Self-Review

**Spec coverage:**
- mise tooling → Task 1 ✅
- compose profiles + anchors + mkcert/caddy → Task 1 ✅
- delete Makefile / README → Task 1 ✅
- GitHub Actions → Task 2 (lint/audit) + Task 10 (test) ✅
- goose migrations = sqlc schema → Task 3 ✅
- sqlc + pgx + Models wrapper → Task 4 ✅
- movies/users/tokens/permissions ported → Tasks 4-7 ✅
- error mapping (ErrRecordNotFound/ErrEditConflict/ErrDuplicateEmail, 23505) → Tasks 4-5 ✅
- remove lib/pq + re-vendor → Task 8 ✅
- unit tests → Task 9 ✅
- DB integration via compose stack, DSN-guarded → Task 10 ✅
- Go 1.22 floor → Task 4 Step 1 ✅

**Known verification points (flagged inline for the implementer):**
- Task 4 Step 8: `NewModels` signature change forces all four model structs to compile in that PR — do the mechanical pgxpool swap for users/tokens/permissions in Task 4, then Tasks 5-7 convert each to sqlc.
- Task 7 Step 3: sqlc param field name for `ANY($2::text[])` may be `Column2` — verify against generated code.
- Task 9: exported names in validator/runtime/filters/jsonlog must be confirmed against source before running.
- Task 10 Step 2: `stdlib.OpenDBFromPool` requires pgx/v5 ≥ v5.4; fallback provided.
```
