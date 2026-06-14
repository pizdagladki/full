# service: matchmaking

Role: WebSocket realtime. Stack — the `go-backend-conventions` skill. Uses: Redis (go-redis).

## Commands (from this folder)
- `make help` — list targets. `make run` / `make test` / `make cover` (≥80%) / `make lint` / `make fmt` / `make mocks`.
- `make docker-up` — bring up the service + Redis locally.

## Responsibility
- Matchmaking queue and player pairing over WebSocket (`coder/websocket`).
- WebSocket signaling handshake: `GET /ws` → accept connection, ping-ack loop (client sends `ping`, server replies `pong`).
- Liveness: `GET /healthz` → `200 {"status":"ok"}`.
- Config: `cmd/config.yaml` locally / env in Docker (`IS_DOCKER`, `HTTP_ADDR`, `REDIS_ADDR`, `REDIS_PASSWORD`); template — `config-example.yaml`.
- New resource — via the `new-resource` skill.

## Gotchas
- Startup pings Redis (via `internal/platform/redis`); a failed ping aborts startup. A local run therefore needs Redis reachable — `make docker-up` brings it up.
- Shared infra comes from `internal/platform/{logger,redis}`; never duplicate it inside the service.
- No Echo dependency — the HTTP surface is a plain `net/http` server in `internal/app/worker_ws.go`.
- `websocket.Accept` uses the secure default (no `InsecureSkipVerify`). `BaseContext` propagates the app context into in-flight WS handlers so cancellation is clean.
