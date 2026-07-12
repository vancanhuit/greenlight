# Let's Go Further - Learn How to Build JSON Web APIs with Go

## Local development

Tools are managed by [mise](https://mise.jdx.dev/). Install mise, then:

```bash
mise install                 # install go, sqlc, goose, golangci-lint
mise run db:up               # start test Postgres (docker compose profile: test)
mise run dev:up              # start dev stack over HTTP
mise run certs:setup         # one-time: local TLS certs via mkcert
mise run dev:up:tls          # dev stack: TLS terminated in the binary
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

## TLS topologies

The API binary can serve HTTPS directly or sit behind a TLS-terminating
reverse proxy (Caddy). TLS behaviour is controlled by four flags:

| Flag | Purpose |
| --- | --- |
| `-tls-cert-file` | PEM certificate used when serving HTTPS directly |
| `-tls-key-file` | PEM private key for the certificate above |
| `-tls-client-ca-file` | CA bundle used to require & verify client certs (mTLS) |
| `-trust-proxy` | Trust `X-Forwarded-*` headers from an upstream proxy |

All HTTPS topologies are exposed on host port **8443**. Run
`mise run certs:setup` once first to generate the local mkcert certificates,
then start the topology you want:

```bash
mise run certs:setup         # one-time: generate local TLS certs

mise run dev:up:tls          # TLS terminated directly in the binary
mise run dev:up:proxy        # TLS terminated at Caddy, plain HTTP to the binary
mise run dev:up:mtls         # mutual TLS between Caddy and the binary
```

Then hit the API over HTTPS:

```bash
curl --cacert .certs/rootCA.pem https://localhost:8443/v1/healthcheck
```

