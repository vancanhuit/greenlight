# TLS Topologies Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add TLS to the Greenlight API across three topologies (direct TLS, proxy-terminated, mTLS) and refine the Compose stacks so each is runnable locally.

**Architecture:** TLS mode is inferred from flags — cert+key enables direct TLS, adding a client CA enables mTLS. A `--trust-proxy` bool gates whether forwarded client-IP headers are honored. Compose gains per-topology profiles and a reusable Caddy base; separate Caddyfiles select proxy vs mTLS upstream behavior.

**Tech Stack:** Go 1.26 stdlib (`crypto/tls`, `crypto/x509`, `net/http`), `github.com/tomasen/realip`, Docker Compose, Caddy 2, mkcert, mise.

## Global Constraints

- Go version floor: `1.26.5` (per `mise.toml`).
- Vendored modules only — no new third-party dependencies (`GOFLAGS=-mod=vendor`). If a dep is added, run `go mod vendor` and commit `vendor/`.
- TLS minimum version: `tls.VersionTLS12`.
- mTLS verification is chain-only (`tls.RequireAndVerifyClientCert`); no CN/SAN identity check.
- Host-facing TLS port is `8443` (never privileged `443`).
- Flags use existing kebab-case naming and `flag` package conventions.
- Conventional Commits for all commit messages.
- Tests run with `mise run test` (no DB) — must pass with `-race`.

---

### Task 1: TLS config flags and boot-time validation

**Files:**
- Modify: `cmd/api/main.go`
- Test: `cmd/api/main_test.go` (create)

**Interfaces:**
- Consumes: existing `config` struct in `cmd/api/main.go`.
- Produces:
  - `config.tls` group: `certFile string`, `keyFile string`, `clientCAFile string`, `trustProxy bool`.
  - `func validateTLSConfig(certFile, keyFile, clientCAFile string) error` — returns non-nil when the flag combination is invalid.

- [ ] **Step 1: Write the failing test**

Create `cmd/api/main_test.go`:

```go
package main

import "testing"

func TestValidateTLSConfig(t *testing.T) {
	tests := []struct {
		name         string
		certFile     string
		keyFile      string
		clientCAFile string
		wantErr      bool
	}{
		{name: "plain http - all empty", wantErr: false},
		{name: "direct tls - cert and key", certFile: "c.pem", keyFile: "k.pem", wantErr: false},
		{name: "mtls - cert key and ca", certFile: "c.pem", keyFile: "k.pem", clientCAFile: "ca.pem", wantErr: false},
		{name: "cert without key", certFile: "c.pem", wantErr: true},
		{name: "key without cert", keyFile: "k.pem", wantErr: true},
		{name: "client ca without cert and key", clientCAFile: "ca.pem", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateTLSConfig(tt.certFile, tt.keyFile, tt.clientCAFile)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateTLSConfig() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/api/ -run TestValidateTLSConfig -v`
Expected: FAIL — `undefined: validateTLSConfig`.

- [ ] **Step 3: Add the `tls` config group**

In `cmd/api/main.go`, add to the `config` struct (after the `cors` group):

```go
	cors struct {
		trustedOrigins []string
	}
	tls struct {
		certFile     string
		keyFile      string
		clientCAFile string
		trustProxy   bool
	}
}
```

- [ ] **Step 4: Implement `validateTLSConfig`**

Add to `cmd/api/main.go` (package-level function):

```go
func validateTLSConfig(certFile, keyFile, clientCAFile string) error {
	if (certFile == "") != (keyFile == "") {
		return errors.New("both -tls-cert-file and -tls-key-file must be set together")
	}
	if clientCAFile != "" && certFile == "" {
		return errors.New("-tls-client-ca-file requires -tls-cert-file and -tls-key-file")
	}
	return nil
}
```

Add `"errors"` to the import block in `cmd/api/main.go`.

- [ ] **Step 5: Register the flags and validate at boot**

In `main()`, after the `cors-trusted-origins` `flag.Func` block and before `displayVersion`:

```go
	flag.StringVar(&cfg.tls.certFile, "tls-cert-file", "", "TLS certificate file (enables direct TLS)")
	flag.StringVar(&cfg.tls.keyFile, "tls-key-file", "", "TLS private key file")
	flag.StringVar(&cfg.tls.clientCAFile, "tls-client-ca-file", "", "CA file to verify client certificates (enables mTLS)")
	flag.BoolVar(&cfg.tls.trustProxy, "trust-proxy", false, "Trust X-Forwarded-For/X-Real-IP headers for client IP")
```

After `flag.Parse()` and the `displayVersion` block, before `logger` is created:

```go
	if err := validateTLSConfig(cfg.tls.certFile, cfg.tls.keyFile, cfg.tls.clientCAFile); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
```

- [ ] **Step 6: Run tests to verify they pass**

Run: `go test ./cmd/api/ -run TestValidateTLSConfig -v`
Expected: PASS (all sub-tests).

- [ ] **Step 7: Verify build**

Run: `go build ./cmd/api/`
Expected: no output, exit 0.

- [ ] **Step 8: Commit**

```bash
git add cmd/api/main.go cmd/api/main_test.go
git commit -m "feat: add TLS config flags and boot validation"
```

---

### Task 2: Serve TLS / mTLS from the binary

**Files:**
- Modify: `cmd/api/server.go`
- Test: `cmd/api/server_test.go` (create)

**Interfaces:**
- Consumes: `config.tls` from Task 1; existing `application.serve()` in `cmd/api/server.go`.
- Produces: `func (app *application) tlsConfig() (*tls.Config, error)` — returns `(nil, nil)` when no cert is configured, otherwise a `*tls.Config` with `MinVersion: tls.VersionTLS12` and, when a client CA is set, `ClientCAs` + `ClientAuth: RequireAndVerifyClientCert`.

- [ ] **Step 1: Write the failing test**

Create `cmd/api/server_test.go`:

```go
package main

import (
	"crypto/tls"
	"os"
	"path/filepath"
	"testing"
)

// minimal PEM CA fixture generated once; any valid CA cert works.
const testCAPEM = `-----BEGIN CERTIFICATE-----
MIIBhTCCASugAwIBAgIQIRi6zePL6mKjOipn+dNuaTAKBggqhkjOPQQDAjASMRAw
DgYDVQQKEwdBY21lIENvMB4XDTE3MTAyMDE5NDMwNloXDTE4MTAyMDE5NDMwNlow
EjEQMA4GA1UEChMHQWNtZSBDbzBZMBMGByqGSM49AgEGCCqGSM49AwEHA0IABD0d
7VNhbWvZLWPuj/RtHFjvtJBEwOkhbN/BnnE8rnZR8+sbwnc/KhCk3FhnpHZnQz7B
5aETbbIgmuvewdjvSBSjYzBhMA4GA1UdDwEB/wQEAwICpDATBgNVHSUEDDAKBggr
BgEFBQcDATAPBgNVHRMBAf8EBTADAQH/MCkGA1UdEQQiMCCCDmxvY2FsaG9zdDo1
NDUzgg4xMjcuMC4wLjE6NTQ1MzAKBggqhkjOPQQDAgNIADBFAiEA2zpJEPQyz6/l
Wf86aX6PepsntZv2GYlA5UpabfT2EZICICpJ5h/iI+i341gBmLiAFQOyTDT+/wQc
6MF9+Yw1Yy0t
-----END CERTIFICATE-----`

func TestTLSConfigDisabledWhenNoCert(t *testing.T) {
	app := &application{}
	cfg, err := app.tlsConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg != nil {
		t.Errorf("expected nil tls.Config when no cert configured, got %v", cfg)
	}
}

func TestTLSConfigSetsMinVersion(t *testing.T) {
	app := &application{}
	app.config.tls.certFile = "cert.pem"
	app.config.tls.keyFile = "key.pem"

	cfg, err := app.tlsConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected non-nil tls.Config")
	}
	if cfg.MinVersion != tls.VersionTLS12 {
		t.Errorf("MinVersion = %d, want %d", cfg.MinVersion, tls.VersionTLS12)
	}
	if cfg.ClientAuth != tls.NoClientCert {
		t.Errorf("ClientAuth = %d, want NoClientCert", cfg.ClientAuth)
	}
}

func TestTLSConfigEnablesMTLS(t *testing.T) {
	dir := t.TempDir()
	caPath := filepath.Join(dir, "ca.pem")
	if err := os.WriteFile(caPath, []byte(testCAPEM), 0o600); err != nil {
		t.Fatal(err)
	}

	app := &application{}
	app.config.tls.certFile = "cert.pem"
	app.config.tls.keyFile = "key.pem"
	app.config.tls.clientCAFile = caPath

	cfg, err := app.tlsConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.ClientAuth != tls.RequireAndVerifyClientCert {
		t.Errorf("ClientAuth = %d, want RequireAndVerifyClientCert", cfg.ClientAuth)
	}
	if cfg.ClientCAs == nil {
		t.Error("expected ClientCAs pool to be set")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/api/ -run TestTLSConfig -v`
Expected: FAIL — `app.tlsConfig undefined`.

- [ ] **Step 3: Implement `tlsConfig`**

Add to `cmd/api/server.go`:

```go
func (app *application) tlsConfig() (*tls.Config, error) {
	if app.config.tls.certFile == "" {
		return nil, nil
	}

	cfg := &tls.Config{MinVersion: tls.VersionTLS12}

	if app.config.tls.clientCAFile != "" {
		caPEM, err := os.ReadFile(app.config.tls.clientCAFile)
		if err != nil {
			return nil, fmt.Errorf("read client CA file: %w", err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(caPEM) {
			return nil, fmt.Errorf("no valid certificates in %s", app.config.tls.clientCAFile)
		}
		cfg.ClientCAs = pool
		cfg.ClientAuth = tls.RequireAndVerifyClientCert
	}

	return cfg, nil
}
```

Add `"crypto/tls"` and `"crypto/x509"` to the import block in `cmd/api/server.go` (`"fmt"` and `"os"` are already imported).

- [ ] **Step 4: Wire it into `serve()`**

In `cmd/api/server.go`, replace the server construction and `ListenAndServe` call. Change the `srv` block and the serve line:

```go
	tlsCfg, err := app.tlsConfig()
	if err != nil {
		return err
	}

	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", app.config.port),
		Handler:      app.routes(),
		IdleTimeout:  time.Minute,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
		ErrorLog:     slog.NewLogLogger(app.logger.Handler(), slog.LevelError),
		TLSConfig:    tlsCfg,
	}
```

Then replace the `err := srv.ListenAndServe()` line (the existing `err` is now assigned, so use `=`):

```go
	app.logger.Info("starting server", "addr", srv.Addr, "env", app.config.env)

	if tlsCfg != nil {
		err = srv.ListenAndServeTLS(app.config.tls.certFile, app.config.tls.keyFile)
	} else {
		err = srv.ListenAndServe()
	}
	if !errors.Is(err, http.ErrServerClosed) {
		return err
	}
```

Note: the `shutdownError` goroutine and the block after it are unchanged. Because `err` is now first declared by `tlsCfg, err := app.tlsConfig()`, the later serve assignment uses `=`.

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./cmd/api/ -run TestTLSConfig -v`
Expected: PASS (3 tests).

- [ ] **Step 6: Verify full package builds and tests pass**

Run: `go build ./cmd/api/ && go test ./cmd/api/ -race`
Expected: build clean, tests PASS.

- [ ] **Step 7: Commit**

```bash
git add cmd/api/server.go cmd/api/server_test.go
git commit -m "feat: serve direct TLS and mTLS from the binary"
```

---

### Task 3: Gate client-IP trust behind `--trust-proxy`

**Files:**
- Modify: `cmd/api/middlewares.go`
- Test: `cmd/api/middlewares_test.go`

**Interfaces:**
- Consumes: `config.tls.trustProxy` from Task 1; `github.com/tomasen/realip` (already vendored).
- Produces: `func (app *application) clientIP(r *http.Request) string` — returns the forwarded IP when `trustProxy` is true, otherwise the host portion of `r.RemoteAddr`.

- [ ] **Step 1: Write the failing test**

Add to `cmd/api/middlewares_test.go`:

```go
func TestClientIP(t *testing.T) {
	tests := []struct {
		name       string
		trustProxy bool
		remoteAddr string
		forwarded  string
		want       string
	}{
		{name: "no trust uses remote addr", trustProxy: false, remoteAddr: "203.0.113.9:5555", forwarded: "70.41.3.18", want: "203.0.113.9"},
		{name: "trust uses forwarded header", trustProxy: true, remoteAddr: "203.0.113.9:5555", forwarded: "70.41.3.18", want: "70.41.3.18"},
		{name: "no trust ignores forwarded", trustProxy: false, remoteAddr: "198.51.100.7:443", forwarded: "70.41.3.18", want: "198.51.100.7"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := &application{}
			app.config.tls.trustProxy = tt.trustProxy

			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.RemoteAddr = tt.remoteAddr
			if tt.forwarded != "" {
				req.Header.Set("X-Forwarded-For", tt.forwarded)
			}

			if got := app.clientIP(req); got != tt.want {
				t.Errorf("clientIP() = %q, want %q", got, tt.want)
			}
		})
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/api/ -run TestClientIP -v`
Expected: FAIL — `app.clientIP undefined`.

- [ ] **Step 3: Implement `clientIP`**

Add to `cmd/api/middlewares.go`:

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

Add `"net"` to the import block in `cmd/api/middlewares.go`.

- [ ] **Step 4: Use `clientIP` in `rateLimit`**

In `cmd/api/middlewares.go`, inside `rateLimit`'s returned handler, replace:

```go
			ip := realip.FromRequest(r)
```

with:

```go
			ip := app.clientIP(r)
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./cmd/api/ -run TestClientIP -v`
Expected: PASS (3 sub-tests).

- [ ] **Step 6: Verify full package builds and tests pass**

Run: `go build ./cmd/api/ && go test ./cmd/api/ -race`
Expected: build clean, tests PASS.

- [ ] **Step 7: Commit**

```bash
git add cmd/api/middlewares.go cmd/api/middlewares_test.go
git commit -m "feat: gate client IP trust behind --trust-proxy"
```

---

### Task 4: Compose profiles, Caddyfiles, certs, mise tasks, README

**Files:**
- Modify: `compose.yaml`
- Create: `Caddyfile.proxy`, `Caddyfile.mtls`
- Delete: `Caddyfile`
- Modify: `mise.toml`
- Modify: `README.md`

**Interfaces:**
- Consumes: the `--tls-cert-file`, `--tls-key-file`, `--tls-client-ca-file`, `--trust-proxy` flags from Tasks 1-3.
- Produces: Compose profiles `dev`, `dev-tls`, `dev-proxy`, `dev-mtls`; mise tasks `dev:up:tls`, `dev:up:proxy`, `dev:up:mtls`.

- [ ] **Step 1: Create `Caddyfile.proxy`**

Create `Caddyfile.proxy` at repo root:

```
localhost:8443 {
    tls /certs/localhost.pem /certs/localhost-key.pem
    reverse_proxy api-proxy:8000
}
```

- [ ] **Step 2: Create `Caddyfile.mtls`**

Create `Caddyfile.mtls` at repo root:

```
localhost:8443 {
    tls /certs/localhost.pem /certs/localhost-key.pem
    reverse_proxy api-mtls:8000 {
        transport http {
            tls
            tls_trust_pool file /certs/rootCA.pem
            tls_client_auth /certs/client.pem /certs/client-key.pem
            tls_server_name localhost
        }
    }
}
```

- [ ] **Step 3: Delete the old `Caddyfile`**

```bash
git rm Caddyfile
```

- [ ] **Step 4: Rewrite `compose.yaml`**

Replace the entire contents of `compose.yaml` with:

```yaml
x-postgres-base: &pg-base
    image: postgres:18
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
        SSL_CERT_FILE: /certs/rootCA.pem
    volumes:
        - ./.certs:/certs:ro

x-api-smtp: &api-smtp
    - "-db-dsn=postgres://dev:dev@postgres-dev:5432/greenlight?sslmode=disable"
    - "-smtp-host=mailpit"
    - "-smtp-port=1025"
    - "-smtp-username=dev"
    - "-smtp-password=dev"
    - "-smtp-sender=Greenlight <no-reply@greenlight.local>"

x-caddy-base: &caddy-base
    image: caddy:2
    ports:
        - "8443:8443"
    volumes:
        - ./.certs:/certs:ro

services:
    postgres-dev:
        <<: *pg-base
        profiles: ["dev", "dev-tls", "dev-proxy", "dev-mtls"]
        environment:
            POSTGRES_USER: dev
            POSTGRES_PASSWORD: dev
            POSTGRES_DB: greenlight
        ports:
            - "5432:5432"
        volumes:
            - pgdata:/var/lib/postgresql
        networks:
            - dev
            - dev-tls
            - dev-proxy
            - dev-mtls

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
            - /var/lib/postgresql
        networks:
            - test

    api:
        <<: *api-base
        profiles: ["dev"]
        command:
            - "-port=8000"
            - "-db-dsn=postgres://dev:dev@postgres-dev:5432/greenlight?sslmode=disable"
            - "-smtp-host=mailpit"
            - "-smtp-port=1025"
            - "-smtp-username=dev"
            - "-smtp-password=dev"
            - "-smtp-sender=Greenlight <no-reply@greenlight.local>"
        ports:
            - "8000:8000"
        depends_on:
            postgres-dev:
                condition: service_healthy
            mailpit:
                condition: service_started
        networks:
            - dev

    api-tls:
        <<: *api-base
        profiles: ["dev-tls"]
        command:
            - "-port=8443"
            - "-db-dsn=postgres://dev:dev@postgres-dev:5432/greenlight?sslmode=disable"
            - "-smtp-host=mailpit"
            - "-smtp-port=1025"
            - "-smtp-username=dev"
            - "-smtp-password=dev"
            - "-smtp-sender=Greenlight <no-reply@greenlight.local>"
            - "-tls-cert-file=/certs/localhost.pem"
            - "-tls-key-file=/certs/localhost-key.pem"
        ports:
            - "8443:8443"
        depends_on:
            postgres-dev:
                condition: service_healthy
            mailpit:
                condition: service_started
        networks:
            - dev-tls

    api-proxy:
        <<: *api-base
        profiles: ["dev-proxy"]
        command:
            - "-port=8000"
            - "-db-dsn=postgres://dev:dev@postgres-dev:5432/greenlight?sslmode=disable"
            - "-smtp-host=mailpit"
            - "-smtp-port=1025"
            - "-smtp-username=dev"
            - "-smtp-password=dev"
            - "-smtp-sender=Greenlight <no-reply@greenlight.local>"
            - "-trust-proxy"
        depends_on:
            postgres-dev:
                condition: service_healthy
            mailpit:
                condition: service_started
        networks:
            - dev-proxy

    api-mtls:
        <<: *api-base
        profiles: ["dev-mtls"]
        command:
            - "-port=8000"
            - "-db-dsn=postgres://dev:dev@postgres-dev:5432/greenlight?sslmode=disable"
            - "-smtp-host=mailpit"
            - "-smtp-port=1025"
            - "-smtp-username=dev"
            - "-smtp-password=dev"
            - "-smtp-sender=Greenlight <no-reply@greenlight.local>"
            - "-tls-cert-file=/certs/localhost.pem"
            - "-tls-key-file=/certs/localhost-key.pem"
            - "-tls-client-ca-file=/certs/rootCA.pem"
            - "-trust-proxy"
        depends_on:
            postgres-dev:
                condition: service_healthy
            mailpit:
                condition: service_started
        networks:
            - dev-mtls

    caddy-proxy:
        <<: *caddy-base
        profiles: ["dev-proxy"]
        volumes:
            - ./.certs:/certs:ro
            - ./Caddyfile.proxy:/etc/caddy/Caddyfile:ro
        depends_on:
            - api-proxy
        networks:
            - dev-proxy

    caddy-mtls:
        <<: *caddy-base
        profiles: ["dev-mtls"]
        volumes:
            - ./.certs:/certs:ro
            - ./Caddyfile.mtls:/etc/caddy/Caddyfile:ro
        depends_on:
            - api-mtls
        networks:
            - dev-mtls

    mailpit:
        image: axllent/mailpit:v1.30.4
        restart: unless-stopped
        profiles: ["dev", "dev-tls", "dev-proxy", "dev-mtls"]
        environment:
            MP_SMTP_AUTH_FILE: /etc/mailpit-auth.conf
            MP_SMTP_AUTH_ALLOW_INSECURE: 1
            MP_SMTP_TLS_CERT: /certs/mailpit.pem
            MP_SMTP_TLS_KEY: /certs/mailpit-key.pem
        volumes:
            - ./mailpit/auth.conf:/etc/mailpit-auth.conf:ro
            - ./.certs:/certs:ro
        ports:
            - "127.0.0.1:8025:8025"
        networks:
            - dev
            - dev-tls
            - dev-proxy
            - dev-mtls

volumes:
    pgdata:

networks:
    dev:
    dev-tls:
    dev-proxy:
    dev-mtls:
    test:
```

Note: the `x-api-smtp` anchor is declared for readability but each service lists its command inline (Compose does not merge sequence anchors into `command` cleanly), so it is safe to omit; it is included only as documentation and may be deleted if `docker compose config` warns about it.

- [ ] **Step 5: Validate the Compose file for every profile**

Run each and confirm no errors:

```bash
docker compose --profile dev config >/dev/null
docker compose --profile dev-tls config >/dev/null
docker compose --profile dev-proxy config >/dev/null
docker compose --profile dev-mtls config >/dev/null
```

Expected: each exits 0 with no stderr. If `x-api-smtp` triggers a warning, delete that anchor block and re-run.

- [ ] **Step 6: Update `mise.toml` certs and tasks**

In `mise.toml`, change the `certs:setup` mkcert line to add backend SANs, and
mint a dedicated client cert for Caddy's mTLS client auth (a ServerAuth-only
leaf cannot be reused for client auth because Go requires the clientAuth EKU;
`client.pem` still chains to `rootCA.pem`, which the binary verifies via
`-tls-client-ca-file`):

```
    "mkcert -cert-file .certs/localhost.pem -key-file .certs/localhost-key.pem localhost 127.0.0.1 api-tls api-mtls",
    "mkcert -client -cert-file .certs/client.pem -key-file .certs/client-key.pem localhost",
```

Replace the `dev:up:https` task with:

```toml
[tasks."dev:up:tls"]
description = "Start dev stack (direct TLS in binary)"
run = "docker compose --profile dev-tls up -d --build --wait"

[tasks."dev:up:proxy"]
description = "Start dev stack (TLS terminated at Caddy)"
run = "docker compose --profile dev-proxy up -d --build --wait"

[tasks."dev:up:mtls"]
description = "Start dev stack (mutual TLS between Caddy and binary)"
run = "docker compose --profile dev-mtls up -d --build --wait"
```

- [ ] **Step 7: Regenerate certs and verify the stacks build**

```bash
mise run certs:setup
mise run dev:up:tls
curl --cacert .certs/rootCA.pem https://localhost:8443/v1/healthcheck
mise run dev:down 2>/dev/null || docker compose --profile dev-tls down
```

Expected: `curl` returns a JSON healthcheck body with HTTP 200.

If no `dev:down` task exists, tear down explicitly:

```bash
docker compose --profile dev-tls down
```

- [ ] **Step 8: Smoke-test proxy and mTLS profiles**

```bash
mise run dev:up:proxy
curl --cacert .certs/rootCA.pem https://localhost:8443/v1/healthcheck
docker compose --profile dev-proxy down

mise run dev:up:mtls
curl --cacert .certs/rootCA.pem https://localhost:8443/v1/healthcheck
docker compose --profile dev-mtls down
```

Expected: both `curl` calls return HTTP 200 JSON. (The mTLS handshake between Caddy and the binary happens internally; a healthy response confirms the client cert was accepted.)

- [ ] **Step 9: Update `README.md`**

Replace any `dev-https` / `dev:up:https` references with the three new profiles/tasks. Add a short "TLS topologies" section documenting:
- the four flags (`-tls-cert-file`, `-tls-key-file`, `-tls-client-ca-file`, `-trust-proxy`),
- the port `8443`,
- one `mise run` command per topology (`dev:up:tls`, `dev:up:proxy`, `dev:up:mtls`),
- that `certs:setup` must run first.

- [ ] **Step 10: Commit**

```bash
git add compose.yaml Caddyfile.proxy Caddyfile.mtls mise.toml README.md
git rm --cached Caddyfile 2>/dev/null || true
git commit -m "feat: add Compose profiles for TLS, proxy, and mTLS topologies"
```

---

## Self-Review Notes

- **Spec coverage:** topology 1 (Tasks 1-2, `dev-tls`), topology 2 (Task 3 + `dev-proxy`), topology 3 (Task 2 mTLS + `dev-mtls` + `Caddyfile.mtls`), compose refinement (Task 4), certs/SANs (Task 4 Step 6), mise/docs (Task 4 Steps 6-9). All spec sections mapped.
- **Type consistency:** `validateTLSConfig`, `tlsConfig`, `clientIP` names are used identically across tasks and tests.
- **Ports:** `8443` host-facing everywhere; `api-mtls` backend TLS on internal `:8000` only.
