pwd = $(shell pwd)

## help: print this help message
.PHONY: help
help:
	@echo 'Usage:'
	@sed -n 's/^##//p' $(MAKEFILE_LIST) | column -t -s ':' |  sed -e 's/^/ /'

.PHONY: confirm
confirm:
	@echo -n "Are you sure? [y/N] " && read ans && [ $${ans:-N} = y ]

## db/create: Start a new database instance in docker for local development
.PHONY: db/create
db/create:
	docker container run --detach \
						 --name db \
						 --restart always \
						 --health-cmd 'pg_isready -d greenlight -U dev' \
						 --health-interval 10s \
						 --health-timeout 5s \
						 --health-retries 5 \
						 --publish 5432:5432 \
						 --env POSTGRES_USER=dev \
						 --env POSTGRES_PASSWORD=dev \
						 --env POSTGRES_DB=greenlight \
						 --mount 'type=bind,src=$(pwd)/sql,dst=/docker-entrypoint-initdb.d' \
						 postgres:14.2

## db/drop: Drop the docker database instance
.PHONY: db/drop
db/drop:
	docker container rm -f db

## db/migrations/new name=$1: create a new database migration
.PHONY: db/migrations/new
db/migrations/new:
	@echo 'Creating migration files for $(name)...'
	migrate create -seq -ext=.sql -dir=$(pwd)/migrations $(name)

## db/migrations/up: apply all up database migrations
.PHONY: db/migrations/up
db/migrations/up: confirm
	migrate \
	-path=$(pwd)/migrations \
	-database=$(GREENLIGHT_DB_DSN) up

## db/migrations/down: apply all down database migrations
.PHONY: db/migrations/down
db/migrations/down:
	migrate \
	-path=$(pwd)/migrations \
	-database=$(GREENLIGHT_DB_DSN) down

## run/api: run the cmd/api application
.PHONY: run/api
run/api:
	go run ./cmd/api -db-dsn=$(GREENLIGHT_DB_DSN)

## build/api: build the cmd/api application
.PHONY: build/api
build/api:
	go build -ldflags='-s' -o=./bin/api ./cmd/api
	GOOS=linux GOARCH=amd64 go build -ldflags='-s' -o=./bin/linux_amd64/api ./cmd/api

## vendor: tidy and vendor dependencies
.PHONY: vendor
vendor:
	@echo 'Tidying and verifying module dependencies...'
	go mod tidy
	go mod verify
	@echo 'Vendoring dependencies...'
	go mod vendor

## audit: tidy and vendor dependencies and format, vet and test all code
.PHONY: audit
audit: vendor
	@echo 'Formatting code...'
	go fmt ./...
	@echo 'Vetting code...'
	go vet ./...
	staticcheck ./...
	@echo 'Running tests...'
	go test -race -vet=off ./...
