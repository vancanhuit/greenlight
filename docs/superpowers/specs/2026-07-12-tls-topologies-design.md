# TLS Topologies Design

Date: 2026-07-12

## Goal

Add TLS support to the Greenlight API across three deployment topologies, and
refine the Compose stacks so each topology is runnable locally:

1. **Direct TLS** — the binary terminates TLS itself using `--tls-cert-file` and
   `--tls-key-file`.
2. **Proxy terminates TLS** — a reverse proxy (Caddy) terminates TLS and the
   binary runs plain HTTP. The binary is proxy-aware and preserves the real
   client IP.
3. **Mutual TLS (mTLS)** — both the reverse proxy and the binary run TLS; the
   binary authenticates the proxy via a TLS client certificate. The existing
   mkcert server certificate / shared CA is reused for this mode.

## Decisions

- **Mode selection (option C — hybrid inference):** no explicit mode flag.
  TLS on/off inferred from cert+key presence; mTLS auto-enables when a client CA
  file is supplied.
- **Client IP trust (option B — `--trust-proxy` bool):** forwarded headers are
  honored only when `--trust-proxy` is set. Direct-exposed modes use
  `RemoteAddr` and cannot be spoofed.
- **mTLS verification (option A — chain-only):** Go
  `tls.RequireAndVerifyClientCert` against the CA pool. No identity (CN/SAN)
  assertion. Acceptable for the dev/reference setup with a single trusted proxy
  and a shared mkcert CA.
- **Compose:** rename `dev-https` profile to `dev-proxy`. One reusable `caddy`
  base via a YAML anchor; per-topology thin services override volume mounts to
  select their Caddyfile. Separate `Caddyfile.proxy` and `Caddyfile.mtls`.
- **Ports:** host-facing TLS on `8443` (not privileged `443`).

## 1. Config and flags (`cmd/api/main.go`)

Add a `tls` group to `config`:

```go
tls struct {
    certFile     string
    keyFile      string
    clientCAFile string
    trustProxy   bool
}
```

Flags:

| Flag | Default | Purpose |
|------|---------|---------|
| `--tls-cert-file` | `""` | PEM server certificate |
| `--tls-key-file` | `""` | PEM private key |
| `--tls-client-ca-file` | `""` | CA pool to verify client certs (enables mTLS) |
| `--trust-proxy` | `false` | Honor `X-Forwarded-For` / `X-Real-IP` |

Boot-time validation (fatal on violation):

- `certFile` set XOR `keyFile` set — both required together.
- `clientCAFile` set while `certFile`/`keyFile` empty — mTLS requires server TLS.

Resolved mode:

- neither cert nor key — plain HTTP.
- cert + key — direct TLS.
- cert + key + client CA — mTLS.

## 2. Server (`cmd/api/server.go`)

Build a `*tls.Config` only when a cert+key are configured:

- `MinVersion: tls.VersionTLS12`.
- When `clientCAFile` is set: load the PEM into an `*x509.CertPool`, assign to
  `ClientCAs`, and set `ClientAuth: tls.RequireAndVerifyClientCert`.

Assign the config to `srv.TLSConfig`. Serve branch:

- TLS configured — `srv.ListenAndServeTLS(certFile, keyFile)`.
- otherwise — `srv.ListenAndServe()`.

Graceful shutdown is unchanged: both serve calls return `http.ErrServerClosed`
on shutdown, and the existing signal/`Shutdown` path already handles that.

## 3. Client IP gating (`cmd/api/middlewares.go`)

Add a helper:

```go
func (app *application) clientIP(r *http.Request) string {
    if app.config.tls.trustProxy {
        return realip.FromRequest(r)
    }
    host, _, err := net.SplitHostPort(r.RemoteAddr)
    if err != nil {
        return r.RemoteAddr
    }
    return host
}
```

`rateLimit` calls `app.clientIP(r)` instead of `realip.FromRequest(r)` directly.
Proxy and mTLS profiles pass `--trust-proxy`; direct modes do not, closing the
IP-spoofing gap on directly reachable deployments.

## 4. Compose (`compose.yaml`)

Profiles:

| Profile | Binary service | Binary listen | Caddy | Binary flags |
|---------|----------------|---------------|-------|--------------|
| `dev` | `api` | plain `:8000` | — | (none) |
| `dev-tls` | `api-tls` | TLS `:8443` | — | `--tls-cert-file --tls-key-file` |
| `dev-proxy` | `api-proxy` | plain `:8000` | terminates `:8443` | `--trust-proxy` |
| `dev-mtls` | `api-mtls` | TLS `:8000` | terminate + client cert to backend | `--tls-cert-file --tls-key-file --tls-client-ca-file=/certs/rootCA.pem --trust-proxy` |

Notes:

- `dev-tls` maps host `8443` to the binary directly; no Caddy.
- `dev-mtls` backend TLS on `:8000` is internal only (not host-exposed); the
  public edge stays on `8443` via Caddy.
- `api-proxy` replaces the current `api-https` service.

Reusable Caddy base via a YAML anchor; per-topology services override `volumes`
to mount their Caddyfile (the `<<:` merge does not deep-merge the `volumes`
list, so each service redeclares the full list):

```yaml
x-caddy-base: &caddy-base
    image: caddy:2
    ports:
        - "8443:8443"
    volumes:
        - ./.certs:/certs:ro

services:
    caddy-proxy:
        <<: *caddy-base
        profiles: ["dev-proxy"]
        volumes:
            - ./.certs:/certs:ro
            - ./Caddyfile.proxy:/etc/caddy/Caddyfile:ro
        depends_on: [api-proxy]
        networks: [dev-proxy]

    caddy-mtls:
        <<: *caddy-base
        profiles: ["dev-mtls"]
        volumes:
            - ./.certs:/certs:ro
            - ./Caddyfile.mtls:/etc/caddy/Caddyfile:ro
        depends_on: [api-mtls]
        networks: [dev-mtls]
```

Networks add `dev-tls`, `dev-proxy`, `dev-mtls` (replacing `dev-https`);
`postgres-dev` and `mailpit` join the new profiles as needed.

## 5. Caddyfiles

`Caddyfile.proxy` (topology 2 — terminate, plain HTTP upstream):

```
localhost:8443 {
    tls /certs/localhost.pem /certs/localhost-key.pem
    reverse_proxy api-proxy:8000
}
```

`Caddyfile.mtls` (topology 3 — terminate public TLS, present client cert to
backend over TLS):

```
localhost:8443 {
    tls /certs/localhost.pem /certs/localhost-key.pem
    reverse_proxy api-mtls:8000 {
        transport http {
            tls
            tls_trust_pool file /certs/rootCA.pem
            tls_client_auth /certs/localhost.pem /certs/localhost-key.pem
            tls_server_name localhost
        }
    }
}
```

Caddy reuses `localhost.pem` as its client certificate (chains to `rootCA.pem`),
matching the "reuse the TLS server certificate" requirement. `tls_server_name
localhost` makes Caddy's upstream verification match the backend certificate's
SAN.

## 6. Certificates (`certs:setup` mise task)

Extend the mkcert server certificate SAN list so Caddy's upstream verification
matches the backend service hostnames:

```
mkcert -cert-file .certs/localhost.pem -key-file .certs/localhost-key.pem \
    localhost 127.0.0.1 api-tls api-mtls
```

No new certificate files are introduced — `localhost.pem` doubles as Caddy's
mTLS client certificate. `rootCA.pem` is already copied by the existing task and
serves as the binary's `--tls-client-ca-file`.

## 7. mise tasks (`mise.toml`)

- Rename `dev:up:https` → `dev:up:proxy` (`docker compose --profile dev-proxy`).
- Add `dev:up:tls` (`--profile dev-tls`).
- Add `dev:up:mtls` (`--profile dev-mtls`).
- `certs:setup` gains the extra SANs above.

## 8. Documentation

Update `README.md`: replace `dev-https` references with `dev-proxy`, document
the three topologies, the new flags, and the `8443` port.

## Testing

- Unit: `clientIP` helper — returns `RemoteAddr` host when `trustProxy` is
  false, returns forwarded value when true. Table-driven in
  `cmd/api/middlewares_test.go`.
- Unit: boot validation — cert without key (and vice versa) and client CA
  without cert+key are rejected. Extract validation into a testable function.
- Manual/integration: bring up each profile and curl the healthcheck:
  - `dev` — `http://localhost:8000/v1/healthcheck`.
  - `dev-tls` — `https://localhost:8443/v1/healthcheck` (mkcert-trusted).
  - `dev-proxy` — `https://localhost:8443/v1/healthcheck`.
  - `dev-mtls` — `https://localhost:8443/v1/healthcheck`; a direct call to the
    backend without a client cert is rejected.

## Out of scope

- Automatic certificate rotation / reload without restart.
- ACME / public CA issuance (dev uses mkcert).
- TLS client-certificate identity (CN/SAN) enforcement — deferred; chain-only
  is the chosen verification depth.
- Production hardening of cipher suites beyond `MinVersion` TLS 1.2.
