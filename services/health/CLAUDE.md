# service: health

Role: HTTP API. Stack — the `go-backend-conventions` skill. Uses: none (no Postgres/Redis/MinIO).

## Commands (from this folder)
- `make help` — list all targets. `make run` / `make test` / `make build` / `make vet`.
- `make cover` (≥80%) / `make lint` / `make fmt` / `make mocks` — quality gates + codegen.

## Responsibility
- Liveness endpoint: `GET /v1/health` → `200 {"status":"ok"}`.
- Config: `cmd/config.yaml` locally / env (`HTTP_ADDR`, `IS_DOCKER`) in Docker; template — `config-example.yaml`.

## Gotchas
- Pure stdlib `net/http` + zap; no datastore. Add a resource via the `new-resource` skill if this grows.
