# Utilize latest Go / tooling features — design

Date: 2026-07-04

Improve the greenlight codebase by adopting current Go and tooling capabilities.
Delivered as three independent PRs off `main`.

## PR A — Embed migrations, run them at boot via goose provider

**Goal:** ship migrations inside the binary and apply pending ones automatically
on every API start, safely across concurrent instances.

- `migrations/embed.go`: `package migrations`, `//go:embed *.sql`, `var FS embed.FS`.
  sqlc keeps reading the `migrations/` directory for schema; it ignores `.go`
  files, so `sqlc.yaml` is unchanged.
- Migration runner in `cmd/api`:
  - `sqlDB := stdlib.OpenDBFromPool(pool)` — the wrapper's `Close()` does not
    close the underlying `pgxpool.Pool` (verified by the existing test), so the
    pool stays usable by the app.
  - `locker, err := lock.NewPostgresSessionLocker()` — Postgres advisory
    session lock so only one instance migrates at a time.
  - `p, err := goose.NewProvider(goose.DialectPostgres, sqlDB, migrations.FS, goose.WithSessionLocker(locker))`
  - `results, err := p.Up(ctx)` — log applied migrations.
- Call the runner in `main()` after `openDB`, before `serve()`. A migration
  error is fatal (`PrintFatal`) — fail fast rather than serve on a bad schema.
- Port `internal/data/main_test.go` off the legacy global API
  (`goose.SetDialect` / `goose.Up`) to the same provider + embedded `FS`, so
  tests and runtime share one migration path.
- Keep the `migrate:up|down|new` mise CLI tasks as developer tools.

## PR B — Inject version via linker flags, remove vcs.go

- `main.go`: `var version = "dev"`, overridden at build time via
  `-ldflags "-X main.version=..."`. Remove the `vcs` import and delete
  `internal/vcs/vcs.go`.
- Build task: `-ldflags="-s -X main.version=$(git describe --tags --always --dirty)"`.

## PR C — Update dependencies, review sqlc features

- `go get -u ./...` within current majors, then `go mod tidy && go mod vendor`.
  Concrete bumps: `chi v5.3.0`, `x/time v0.15.0`, `x/crypto v0.53.0`,
  `x/text v0.38.0`. `pgx`, `goose`, `x/sync` already latest.
- Verify `go build`, `go vet`, `golangci-lint run ./...`, unit suite.
- sqlc / goose / golangci-lint CLI tool versions in `mise.toml` reviewed; bump
  any that trail latest. Notable sqlc latest-version features captured in the
  PR description.

## Cross-cutting

- **Error handling:** boot migration failure is fatal.
- **Testing:** the ported `TestMain` exercises the embedded-FS provider path; no
  new dedicated tests required.
- **Workflow:** each PR = worktree off `main`, build/vet/lint/test, push,
  verify CI green, squash-merge, delete branch.
