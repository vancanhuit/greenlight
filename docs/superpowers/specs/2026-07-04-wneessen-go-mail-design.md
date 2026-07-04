# Replace go-mail/mail/v2 with wneessen/go-mail — design

Date: 2026-07-04

Swap the unmaintained `github.com/go-mail/mail/v2` (a gomail fork) for the
actively maintained `github.com/wneessen/go-mail`. Delivered as one PR.

## Scope

Only `internal/mailer/mailer.go` changes plus `go.mod`/`go.sum`/`vendor`. The
`Send` interface, the embedded `templates/` FS, and the template-rendering
logic are unchanged, so `internal/mailer/mailer_test.go` still passes as-is.

## API mapping

| Current (`go-mail/mail/v2`) | wneessen/go-mail |
| --- | --- |
| `mail.NewDialer(host, port, user, pass)` + `.Timeout` | `mail.NewClient(host, opts...)` |
| `mail.NewMessage()` | `mail.NewMsg()` |
| `msg.SetHeader("To"/"From"/"Subject", v)` | `msg.To(v)` / `msg.From(v)` / `msg.Subject(v)` |
| `msg.SetBody("text/plain", b)` | `msg.SetBodyString(mail.TypeTextPlain, b)` |
| second `msg.SetBody("text/html", b)` | `msg.AddAlternativeString(mail.TypeTextHTML, b)` |
| `dialer.DialAndSend(msg)` | `client.DialAndSend(msg)` |

## Implementation

- `Mailer` struct stores plain fields: `host`, `port`, `username`, `password`,
  `sender`. `New(host, port, username, password, sender) Mailer` keeps its
  current signature (no error return), so `cmd/api/main.go` is untouched.
- `Send` builds a fresh `*mail.Client` on each call — `NewClient` does not dial
  until `DialAndSend`, mirroring the current one-connection-per-send behavior:
  ```
  client, err := mail.NewClient(m.host,
      mail.WithPort(m.port),
      mail.WithTimeout(5*time.Second),
      mail.WithTLSPolicy(mail.TLSOpportunistic),
      mail.WithSMTPAuth(mail.SMTPAuthAutoDiscover),
      mail.WithUsername(m.username),
      mail.WithPassword(m.password),
  )
  ```
- Build the message: `mail.NewMsg()`, then `msg.To(recipient)`,
  `msg.From(m.sender)` (both return errors — handle them), `msg.Subject(...)`,
  `msg.SetBodyString(mail.TypeTextPlain, plainBody.String())`,
  `msg.AddAlternativeString(mail.TypeTextHTML, htmlBody.String())`.
- Keep the retry loop: 3 attempts, 1s sleep between, return the last error.

## Decisions

- **SMTP auth:** `SMTPAuthAutoDiscover` — the server advertises supported
  mechanisms and go-mail selects the strongest, closest to the old
  auto-negotiating dialer.
- **TLS:** `TLSOpportunistic` — STARTTLS when offered, plaintext otherwise,
  matching the previous dialer behavior on port 25.

## Error handling & testing

- `NewClient` and the `To`/`From` setters return errors; surface them from
  `Send`. Delivery failures keep the existing retry-then-return semantics.
- `go build`, `go vet`, `golangci-lint run ./...`, and the full unit suite
  (including the unchanged mailer template test) must pass. CI does not send
  live mail, so auth/TLS wiring is verified by compilation and code review.
