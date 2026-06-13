# service: health

Role: HTTP API. Stack — the `go-backend-conventions` skill. Uses: none (no Postgres/Redis/MinIO).

## Commands (from this folder)
- `make run` / `make test` / `make vet` / `make build` / `make lint`

## Responsibility
- Liveness endpoint: `GET /v1/health` → `200 {"status":"ok"}`.
- Config: `cmd/config.yaml` locally / env (`HTTP_ADDR`, `IS_DOCKER`) in Docker; template — `config-example.yaml`.

## Gotchas
- Pure stdlib `net/http` + zap; no datastore. Add a resource via the `new-resource` skill if this grows.
