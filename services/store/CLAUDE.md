# service: store

Role: HTTP API. Stack — the `go-backend-conventions` skill. Uses: Postgres (pgxpool) + Redis (go-redis).

## Commands (from this folder)
- `make help` — list targets. `make run` / `make test` / `make cover` (>=80%) / `make lint` / `make fmt` / `make mocks`.
- `make migrate` — apply golang-migrate migrations (added once the service owns Postgres tables).
- `make docker-up` — bring up the service + Postgres + Redis + MinIO locally.

## Responsibility
- Catalog browsing, purchase flows, and inventory management (Stripe integration via a PaymentProvider
  interface added by downstream resource slices — the scaffold only wires the infra and a liveness probe).
- Liveness: `GET /healthz` -> `200 {"status":"ok"}`.
- Config: `cmd/config.yaml` locally / env in Docker (`IS_DOCKER`, `HTTP_ADDR`, `POSTGRES_DSN`,
  `REDIS_ADDR`, `REDIS_PASSWORD`); template — `config-example.yaml`; default HTTP addr `:8083`.
- New resource — via the `new-resource` skill.

## Gotchas
- Startup pings both Postgres and Redis (via `internal/platform/{postgres,redis}`); a failed ping aborts
  startup. A local run therefore needs both reachable — `make docker-up` brings them up.
- Shared infra comes from `internal/platform/{logger,postgres,redis}`; never duplicate it inside the service.
- No migrations directory in the initial scaffold — SQL schema is added by the first resource slice.
