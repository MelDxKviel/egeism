# egeism — developer entrypoints (§6 WS-F).
# On Windows without `make`, run the underlying commands from README.md.

COMPOSE ?= docker compose -f deploy/docker-compose.yml
DATABASE_URL ?= postgres://egeism:egeism@localhost:5432/egeism?sslmode=disable
SEED ?= deploy/seed/tasks.sample.json

.PHONY: help
help:
	@echo "Targets: dev up down migrate sqlc test build ingest tidy"

## dev: bring up infra (postgres/redis/minio) only, for local `go run`.
.PHONY: dev
dev:
	$(COMPOSE) up -d postgres redis minio minio-init

## up: build and run the whole stack.
.PHONY: up
up:
	$(COMPOSE) up --build -d

## down: stop the stack (keep volumes).
.PHONY: down
down:
	$(COMPOSE) down

## migrate: apply DB migrations against DATABASE_URL.
.PHONY: migrate
migrate:
	DATABASE_URL="$(DATABASE_URL)" go run ./cmd/migrate up

## sqlc: regenerate type-safe DB code from queries.
.PHONY: sqlc
sqlc:
	sqlc generate

## test: run all Go tests (checker safety-net included).
.PHONY: test
test:
	go test ./...

## build: compile all binaries into ./bin.
.PHONY: build
build:
	go build -o bin/ ./cmd/...

## ingest: load the seed dataset as draft tasks.
.PHONY: ingest
ingest:
	DATABASE_URL="$(DATABASE_URL)" go run ./cmd/ingest -source file -provider dataset-demo -path $(SEED)

## tidy: sync go.mod/go.sum.
.PHONY: tidy
tidy:
	go mod tidy
