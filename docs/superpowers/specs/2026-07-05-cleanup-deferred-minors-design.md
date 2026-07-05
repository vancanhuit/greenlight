# Cleanup: Deferred Minors from Store/Emailer Seams — Design

Date: 2026-07-05
Follows: PR #17 (`refactor: Store/Emailer seams`, commit `17d188b`)

## Goal

Remove dead code the seams refactor left behind. No behavior change — one
interface-line removal plus test-only cleanup. Scope A: strip the fakes to
exactly what the tests use.

## Changes

### 1. Drop unused `TokenStore.Insert`
- `cmd/api/stores.go`: remove `Insert(token *data.Token) error` from the
  `TokenStore` interface. No handler calls `app.tokens.Insert`; `TokenModel.New`
  calls `Insert` internally, below the seam, so the interface does not need it.
- `cmd/api/fakes_test.go`: remove the matching `Insert` method from
  `fakeTokenStore`.

### 2. Replace `equalStrings` with `slices.Equal`
- `internal/data/movies_db_test.go`: delete the hand-rolled `equalStrings`
  helper and call `slices.Equal` at its call sites. Add the `slices` import.
  Module is `go 1.26`, so `slices.Equal` is available.

### 3. Strip unused fake scaffolding
- `cmd/api/fakes_test.go`: remove every error-injection field that no test
  assigns — movie `insertErr`/`getErr`/`getAllErr`/`deleteErr`, and the user,
  token, and permission equivalents, plus the `fakeTokenStore.deleted` slice
  (appended but never asserted). Keep `fakeMovieStore.updateErr` — the 409
  edit-conflict test sets it (`movies_test.go:191`).
- Remove the now-dead `id < 1` guards in `fakeMovieStore.Get` and
  `fakeMovieStore.Delete`: `readIDParam` rejects ids `< 1` before the handler
  reaches the store, and a map miss already returns `ErrRecordNotFound`.

## Non-goals

- No production behavior change. No new tests. No interface changes beyond
  removing the single unused `TokenStore.Insert` method.

## Testing

- `go build ./...`, `go vet ./...`, `go test ./...` green.
- `go test -race ./cmd/api/...` clean.
- Data-layer tests (incl. the `slices.Equal` call sites) verified against the
  test DB (`GREENLIGHT_TEST_DB_DSN`, compose `postgres-test`, port 5433).

## Risks

- Removing a fake field the tests actually use would break the build — mitigated
  by verifying each removed field has no assignment in any `_test.go` (only
  `updateErr` is assigned; `deleted` is appended but never read).

## Execution

One branch, one PR, squash-merge (standard workflow). Small enough for a single
commit; no subagent ceremony required.
