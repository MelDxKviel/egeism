# egeism — developer entrypoints (§6 WS-F).
# On Windows without `make`, run the underlying commands from README.md.

COMPOSE ?= docker compose -f deploy/docker-compose.yml
# Production stack: standalone compose + secrets from deploy/.env (DOMAIN, ACME_EMAIL, …).
PROD_COMPOSE ?= docker compose -f deploy/docker-compose.prod.yml --env-file deploy/.env
DATABASE_URL ?= postgres://egeism:egeism@localhost:5432/egeism?sslmode=disable
SEED ?= deploy/seed/tasks.sample.json

.PHONY: help
help:
	@echo "Dev:  dev up down migrate sqlc test build ingest tidy"
	@echo "Prod: prod-config prod-up prod-down prod-logs prod-ps"

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

# ── Production (run ON THE SERVER; needs deploy/.env, see deploy/.env.prod.example) ──

## prod-config: validate the prod compose + env interpolation without starting anything.
.PHONY: prod-config
prod-config:
	$(PROD_COMPOSE) config >/dev/null && echo "prod compose OK"

## prod-up: build + start the whole prod stack (Caddy edge, auto-TLS). Idempotent — also the deploy step.
.PHONY: prod-up
prod-up:
	$(PROD_COMPOSE) up --build -d

## prod-down: stop the prod stack (keeps volumes: DB, MinIO, Caddy certs).
.PHONY: prod-down
prod-down:
	$(PROD_COMPOSE) down

## prod-logs: tail all prod service logs.
.PHONY: prod-logs
prod-logs:
	$(PROD_COMPOSE) logs -f --tail=100

## prod-ps: show prod service status.
.PHONY: prod-ps
prod-ps:
	$(PROD_COMPOSE) ps
