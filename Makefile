pwd = $(shell pwd)

.PHONY: db
db:
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
