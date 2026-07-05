# Replace `jsonlog` with stdlib `log/slog`

Date: 2026-07-05

## Goal

Delete the custom `internal/jsonlog` package and use Go's standard-library
`log/slog` JSON logger throughout the application. Fewer bespoke lines,
idiomatic structured logging, zero third-party dependency.

## Decisions

1. **Native slog output** — accept slog's JSON shape. No custom handler or
   `ReplaceAttr` to preserve the old keys.
2. **Fatal inline** — no `PrintFatal` wrapper. Fatal sites do
   `logger.Error(...)` then `os.Exit(1)` directly.
3. **No stack trace on fatal** — drop the old `debug.Stack()` behaviour. Fatal
   sites are startup failures (DB connect, migrate, serve); Go already surfaces
   traces on real crashes.

## Logger construction

In `cmd/api/main.go`:

```go
logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
```

`nil` handler options → default minimum level `Info` (matches old
`jsonlog.LevelInfo`).

## Field type change

`application.logger` changes from `*jsonlog.Logger` to `*slog.Logger`.

## Call-site mapping

| Old | New |
| --- | --- |
| `logger.PrintInfo(msg, props)` | `logger.Info(msg, k, v, …)` |
| `logger.PrintError(err, props)` | `logger.Error(err.Error(), k, v, …)` |
| `logger.PrintFatal(err, nil)` | `logger.Error(err.Error()); os.Exit(1)` |

Property maps become flat slog key/value argument pairs, e.g.
`map[string]string{"signal": s.String()}` → `"signal", s.String()`.

Affected files: `cmd/api/main.go`, `cmd/api/server.go`, `cmd/api/errors.go`,
`cmd/api/helpers.go`, `cmd/api/migrate.go`, `cmd/api/users.go`.

## `http.Server.ErrorLog`

The old logger implemented `io.Writer`, so `server.go` used
`log.New(app.logger, "", 0)`. `*slog.Logger` is not an `io.Writer`. Replace
with a bridge that routes stdlib log output through the slog handler at error
level:

```go
ErrorLog: slog.NewLogLogger(app.logger.Handler(), slog.LevelError),
```

## Test harness

`cmd/api/fakes_test.go` builds a discard logger:

```go
// old
logger: jsonlog.New(io.Discard, jsonlog.LevelOff),
// new
logger: slog.New(slog.NewJSONHandler(io.Discard, nil)),
```

## Deletions

- `internal/jsonlog/jsonlog.go`
- `internal/jsonlog/jsonlog_test.go`

(Entire `internal/jsonlog` package removed.)

## Accepted output-shape changes

- `message` key → `msg`
- nested `properties.*` → flat top-level attributes
- `time` format → RFC3339Nano (slog default)
- `level` values unchanged in spirit (`INFO`/`ERROR`); no `FATAL` level
- no `trace` attribute

## Testing

- `go build ./...`, `go vet ./...`, `go test ./...` stay green.
- `go test -race ./cmd/api/...` clean.
- Data-layer tests pass against the test DB (port 5433).
- No new logger unit tests (stdlib is trusted). Handler tests are unaffected —
  the logger is discarded in the test harness.

## Out of scope

- Configurable log level / flags.
- Log sampling, rotation, or additional handlers.
- Changing any log message text or which events are logged.
