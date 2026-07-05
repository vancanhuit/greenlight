# slog Logging Refactor Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the custom `internal/jsonlog` package with stdlib `log/slog` JSON logging.

**Architecture:** Swap `*jsonlog.Logger` for `*slog.Logger` on the `application` struct, map every `PrintInfo/PrintError/PrintFatal` call to slog equivalents, bridge `http.Server.ErrorLog` via `slog.NewLogLogger`, then delete the `jsonlog` package. This is one atomic task: removing `jsonlog` breaks compilation until every call-site is migrated.

**Tech Stack:** Go 1.26, `log/slog`.

## Global Constraints

- Native slog JSON output; no custom handler / `ReplaceAttr`.
- Fatal handled inline: `logger.Error(...); os.Exit(1)`. No wrapper, no stack trace.
- Default minimum level Info (`slog.NewJSONHandler(w, nil)`).
- No log message text changes; no new logger unit tests.

---

### Task 1: Migrate all logging to slog and delete `jsonlog`

**Files:**
- Modify: `cmd/api/main.go` — logger construction, `application.logger` field type, 3 fatal + 2 info sites.
- Modify: `cmd/api/server.go` — `ErrorLog` bridge + 4 info sites.
- Modify: `cmd/api/errors.go` — `PrintError` → `Error`.
- Modify: `cmd/api/helpers.go` — `PrintError` → `Error`.
- Modify: `cmd/api/migrate.go` — `PrintInfo` → `Info`.
- Modify: `cmd/api/users.go` — `PrintError` → `Error`.
- Modify: `cmd/api/fakes_test.go` — discard logger construction + drop `jsonlog` import.
- Delete: `internal/jsonlog/jsonlog.go`, `internal/jsonlog/jsonlog_test.go`.

**Interfaces:**
- `application.logger` becomes `*slog.Logger`.
- Construction: `slog.New(slog.NewJSONHandler(os.Stdout, nil))`.
- `ErrorLog: slog.NewLogLogger(app.logger.Handler(), slog.LevelError)`.

**Call mapping:**
- `logger.PrintInfo(msg, props)` → `logger.Info(msg, k, v, …)`
- `logger.PrintError(err, props)` → `logger.Error(err.Error(), k, v, …)`
- `logger.PrintFatal(err, nil)` → `logger.Error(err.Error()); os.Exit(1)`
- prop maps → flat key/value pairs (e.g. `"signal", s.String()`).

- [ ] **Step 1: Edit all `cmd/api` files** — field type, construction, call-site mappings, ErrorLog bridge, imports (`log/slog`, drop `jsonlog`; check `log`, `os` usage).
- [ ] **Step 2: Edit `fakes_test.go`** — discard logger, drop `jsonlog` import.
- [ ] **Step 3: Delete the `internal/jsonlog` package** (both files).
- [ ] **Step 4: `go build ./... && go vet ./...`** — Expected: clean.
- [ ] **Step 5: Verify no residual `jsonlog` refs** — `grep -rn jsonlog cmd internal` returns nothing (docs excluded).
- [ ] **Step 6: Start test DB + `go test ./...` and `go test -race ./cmd/api/...`** — Expected: all pass.
- [ ] **Step 7: Commit.**
