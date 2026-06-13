---
name: new-service
description: Scaffold a new Go microservice under services/ following the layered backend canon.
---
Scaffold `services/<name>/` strictly per the `go-backend-conventions` skill (read it first). One service =
one bounded context, exposing HTTP, WebSocket, or running purely as a background worker. Files are
`snake_case.go`; constructors `New<Type>(deps...)` return the layer's interface; implementation structs stay
lowercase; all app code lives under `internal/`.

## Steps
1. Create `services/<name>/`:
   - `cmd/main.go` — `app.New("<name>")` + `app.Run(ctx)` with a graceful-shutdown context (SIGTERM/SIGINT/SIGHUP).
   - `cmd/config-example.yaml` (committed) + `cmd/config.yaml` (gitignored).
   - `internal/app/` — `app.go` (struct App + New + Run + call order) and ONE init file per dependency the
     service actually uses: `init_postgres.go`, `init_redis.go`, `init_storage.go`; plus
     `register_http_routes.go` + `worker_http.go` for an HTTP service, and/or `worker_ws.go` for a realtime
     (gorilla/websocket) service; `run_workers.go` launches the workers and any background loops.
   - `internal/api/{delivery,service,repository,domain,middleware}/` — per-layer interface files
     (`delivery.go` / `service.go` / `repository.go`); add `middleware/auth_middleware.go` (RequireAuth →
     validates the Redis session) if the service is authenticated.
   - `internal/config/{config.go,validate.go}` — only the sections the service needs (Postgres / Redis /
     MinIO / Stripe / OAuth); env-vs-yaml chosen by `IS_DOCKER`; `ValidateConfig` (validator/v10) at startup.
   - `migrations/` — golang-migrate `NNNN_<name>.up.sql` / `.down.sql` if the service owns Postgres tables.
   - `Makefile`, `Dockerfile`, `CLAUDE.md` (templates below).
2. Imports: service-private → `github.com/pizdagladki/full/services/<name>/internal/...`; shared infra →
   `github.com/pizdagladki/full/internal/platform/{logger,postgres,redis,storage}` — never duplicate it.
3. The root `Makefile` aggregates via `$(wildcard services/*)` — a new directory is picked up automatically,
   no edit needed.
4. Add the first resource with the `new-resource` skill.
5. Verify and show output: `make -C services/<name> build vet test lint` green.

## Templates

### services/<name>/CLAUDE.md
```markdown
# service: <name>

Role: <HTTP API | WebSocket realtime | background worker>. Stack — the `go-backend-conventions` skill.
Uses: <Postgres | Redis | MinIO> (only what it needs).

## Commands (from this folder)
- `make run` / `make test` / `make lint`
- `make migrate` — apply golang-migrate migrations (if this service owns Postgres tables)
- `make docker-up` — bring up the service + Postgres + Redis + MinIO locally

## Responsibility
- <what the service does>
- Config: `cmd/config.yaml` locally / env in Docker; template — `config-example.yaml`
- New resource — via the `new-resource` skill.

## Gotchas
- <specifics of this particular service>
```

### services/<name>/Makefile
```makefile
.PHONY: run build test lint vet migrate docker-up docker-down

run:        ; go run ./cmd
build:      ; go build -o bin/app ./cmd
test:       ; go test ./...
vet:        ; go vet ./...
lint:       ; golangci-lint run
migrate:    ; migrate -path migrations -database "$(POSTGRES_DSN)" up
docker-up:  ; docker compose -f ../../deploy/docker-compose.yml up -d $(notdir $(CURDIR)) postgres redis minio
docker-down:; docker compose -f ../../deploy/docker-compose.yml down
```

### services/<name>/Dockerfile

Default — pure-Go service, distroless static (no shell, smallest/safest):
```dockerfile
# Build from the repo root (single go.mod + internal/ in context):
#   docker build -f services/<name>/Dockerfile -t <name> .
FROM golang:1.26 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /bin/app ./services/<name>/cmd

FROM gcr.io/distroless/static-debian12
COPY --from=build /bin/app /app
ENTRYPOINT ["/app"]
```

Variant — a service that shells out to `ffmpeg` (e.g. WebM→MP4 conversion). `gcr.io/distroless/static` has
neither ffmpeg nor a shell, so use a slim base that ships ffmpeg (CGO stays disabled):
```dockerfile
FROM golang:1.26 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /bin/app ./services/<name>/cmd

FROM debian:bookworm-slim
RUN apt-get update \
    && apt-get install -y --no-install-recommends ffmpeg ca-certificates \
    && rm -rf /var/lib/apt/lists/*
COPY --from=build /bin/app /app
ENTRYPOINT ["/app"]
```
