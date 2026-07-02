# service: koth

Role: HTTP API, King-of-the-Hill. Stack — the `go-backend-conventions` skill. Uses: Postgres (pgxpool) +
Redis (go-redis).

## Commands (from this folder)
- `make help` — list targets. `make run` / `make test` / `make cover` (≥80%) / `make lint` / `make fmt` / `make mocks`.
- `make migrate` — apply golang-migrate migrations (added once the service owns Postgres tables).
- `make docker-up` — bring up the service + Postgres + Redis + MinIO locally.

## Responsibility
- This is currently a bare scaffold: infra wiring (Echo, zap, Postgres, Redis) + a liveness probe. No
  King-of-the-Hill business logic yet — resources (domain/repository/service/delivery) are added by
  downstream slices via the `new-resource` skill.
- Liveness: `GET /healthz` → `200 {"status":"ok"}`.
- Config: `cmd/config.yaml` locally / env in Docker (`IS_DOCKER`, `HTTP_ADDR`, `POSTGRES_DSN`,
  `REDIS_ADDR`, `REDIS_PASSWORD`); template — `config-example.yaml`.
- New resource — via the `new-resource` skill.

## Gotchas
- Startup pings both Postgres and Redis (via `internal/platform/{postgres,redis}`); a failed ping aborts
  startup. A local run therefore needs both reachable — `make docker-up` brings them up.
- Shared infra comes from `internal/platform/{logger,postgres,redis}`; never duplicate it inside the service.
