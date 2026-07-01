# Multi-stage build for any of the egeism binaries. Select which with:
#   docker build -f deploy/Dockerfile --build-arg BIN=api .
# BIN is one of: api, bot, worker, ingest, migrate.
FROM golang:1.26-alpine AS build
ARG BIN=api
WORKDIR /src

# Cache modules first.
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/app ./cmd/${BIN}

FROM alpine:3.20
RUN adduser -D -u 10001 app && apk add --no-cache ca-certificates
USER app
COPY --from=build /out/app /usr/local/bin/app
# Seed data used by the ingest one-shot.
COPY --from=build /src/deploy/seed /seed
ENTRYPOINT ["/usr/local/bin/app"]
