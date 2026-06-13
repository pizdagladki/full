---
name: new-service
description: Scaffold a new Go microservice under services/ following the layered backend canon.
---
Scaffold a new service `services/<name>/` strictly per the `go-backend-conventions` skill (read it first).
One service = one bounded context. Files are `snake_case.go`; constructors `New<Type>(deps...)` return the
layer's interface; only interfaces are exported, implementation structs stay lowercase.

## Steps
1. Create `services/<name>/` with the full layered tree:
   - `cmd/main.go` — build `app.New("<name>")` and `app.Run(ctx)` with a graceful-shutdown context
     (SIGTERM/SIGINT/SIGHUP).
   - `cmd/config-example.yaml` (committed) and `cmd/config.yaml` (gitignored).
   - `internal/app/` — `app.go` (struct App: deps as fields + `New` + `Run` + the init call order),
     `init_database.go`, `register_http_routes.go`, `run_workers.go`, `worker_http.go`.
   - `internal/api/{delivery,service,repository,domain,middleware}/` — the per-layer interface files
     `delivery.go` / `service.go` / `repository.go` (each a `type ( ... )` block); add
     `middleware/auth_middleware.go` if the service needs `RequireAuth`.
   - `internal/config/{config.go,validate.go}` — nested structs with `yaml:"..."` + `validate:"..."`;
     `loadFromEnv` vs `loadFromFile` chosen by `IS_DOCKER`; `ValidateConfig` (validator/v10) at startup.
   - `Makefile`, `Dockerfile`, `CLAUDE.md` (templates below).
2. Imports derive from the single module `github.com/pizdagladki/full`:
   - service-private code → `github.com/pizdagladki/full/services/<name>/internal/...`
   - shared infra → `github.com/pizdagladki/full/internal/platform/...` (logger, DB). Do NOT duplicate it.
3. The root `Makefile` aggregates via `$(wildcard services/*)` — a new directory is picked up
   automatically, no edit needed.
4. Add the first resource with the `new-resource` skill.
5. Verify and show output: `make -C services/<name> build vet test lint` green.

## Templates

### services/<name>/CLAUDE.md
```markdown
# service: <name>

HTTP API on Fiber + MongoDB + zap. Architecture — the `go-backend-conventions` skill.

## Commands (from this folder)
- `make run` / `make test` / `make lint`
- `make docker-up` — bring up the service + DB locally

## Responsibility
- <what the service does>
- Config: `cmd/config.yaml` locally / env in Docker; template — `config-example.yaml`
- New resource — via the `new-resource` skill.

## Gotchas
- <specifics of this particular service>
```

### services/<name>/Makefile
```makefile
.PHONY: run build test lint vet docker-up docker-down

run:        ; go run ./cmd
build:      ; go build -o bin/app ./cmd
test:       ; go test ./...
vet:        ; go vet ./...
lint:       ; golangci-lint run
docker-up:  ; docker compose -f ../../deploy/docker-compose.yml up -d $(notdir $(CURDIR)) mongo
docker-down:; docker compose -f ../../deploy/docker-compose.yml down
```

### services/<name>/Dockerfile (multi-stage; build context = repo root)
```dockerfile
# Build from the repo root so the single go.mod and internal/ are in context:
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
