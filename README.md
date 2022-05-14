# Let's Go Further - Learn How to Build JSON Web APIs with Go

## Local development

Tools:

- [Go 1.18+](https://go.dev/)
- [Docker](https://docs.docker.com/get-docker/)
- [migrate CLI](https://github.com/golang-migrate/migrate/tree/master/cmd/migrate)
- [hey](https://github.com/rakyll/hey)
- [staticcheck](https://staticcheck.io/)

```bash
export GREENLIGHT_DB_DSN=postgres://dev:dev@localhost:5432/greenlight?sslmode=disable
make db/create
make run/api
make db/migrations/up
make db/drop
```
