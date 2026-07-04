# Let's Go Further - Learn How to Build JSON Web APIs with Go

## Local development

Tools are managed by [mise](https://mise.jdx.dev/). Install mise, then:

```bash
mise install                 # install go, sqlc, goose, golangci-lint
mise run db:up               # start test Postgres (docker compose profile: test)
mise run dev:up              # start dev stack over HTTP
mise run certs:setup         # one-time: local TLS certs via mkcert
mise run dev:up:https        # dev stack over HTTPS (caddy + mkcert)
mise run migrate:up "$GREENLIGHT_DB_DSN"   # apply migrations
mise run run                 # run the API locally
mise run test                # unit tests
mise run test:db             # unit + DB integration tests
```

Default DSNs and env vars live in `mise.toml`. To override locally, create a
gitignored `mise.local.toml` (mise merges it automatically), e.g.:

```toml
[env]
GREENLIGHT_DB_DSN = "postgres://dev:dev@localhost:5432/greenlight?sslmode=disable"
```
