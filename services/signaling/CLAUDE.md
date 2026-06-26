# service: signaling

Role: WebSocket realtime signaling. Stack — the `go-backend-conventions` skill. Uses: Redis (go-redis).

## Commands (from this folder)
- `make help` — list targets. `make run` / `make test` / `make cover` (≥80%) / `make lint` / `make fmt` / `make mocks`.
- `make docker-up` — bring up the service + Redis locally.

## Responsibility
- WebRTC SDP/ICE relay between two room peers over WebSocket (`coder/websocket`).
- Liveness: `GET /healthz` → `200 {"status":"ok"}`.
- Config: `cmd/config.yaml` locally / env in Docker (`IS_DOCKER`, `HTTP_ADDR`, `REDIS_ADDR`, `REDIS_PASSWORD`,
  `SIG_SESSION_COOKIE`, `SIG_ROOM_TTL`); template — `config-example.yaml`.
- New resource — via the `new-resource` skill.

## Message protocol (`GET /ws`)

Authentication: the client must present a valid session cookie (`session` by default) set by the auth service.
The session maps to a `userID` stored in Redis under `session:<value>`.

All messages are JSON text frames.

### Client → Server

| Type  | Required fields            | Description                                           |
|-------|----------------------------|-------------------------------------------------------|
| join  | `room_id`                  | Join or re-join a signaling room (max 2 members).     |
| sdp   | `room_id`, `sdp` (+ any)   | Relay an SDP offer or answer verbatim to the peer.    |
| ice   | `room_id`, `candidate`     | Relay an ICE candidate verbatim to the peer.          |

### Server → Client

| Type      | Fields                  | Description                                         |
|-----------|-------------------------|-----------------------------------------------------|
| error     | `error`                 | Sent on validation failure, full room, or bad type. |
| peer_left | —                       | Sent when the other peer disconnects.               |

### Room rules
- A room holds at most **2** members (`room:<roomID>:members` Redis SET, TTL from config).
- A third `join` gets `{"type":"error","error":"room is full"}` and the connection is closed.
- `sdp`/`ice` are forwarded **verbatim** (raw bytes) to the other member only — never echoed, never to other rooms.
- A sender not in the room (in-process hub) gets `{"type":"error","error":"not a member of this room"}`.
- On disconnect the remaining peer receives `{"type":"peer_left"}` and the room is deleted from Redis.

## Redis keys
- `session:<sessionID>` → int64 userID (managed by the auth service)
- `room:<roomID>:members` → Redis SET of base-10 userID strings; TTL = `signaling.room_ttl` (default 30m)

## Gotchas
- Startup pings Redis (via `internal/platform/redis`); a failed ping aborts startup. A local run therefore needs Redis reachable — `make docker-up` brings it up.
- Shared infra comes from `internal/platform/{logger,redis}`; never duplicate it inside the service.
- No Echo dependency — the HTTP surface is a plain `net/http` server in `internal/app/worker_ws.go`.
- `websocket.Accept` uses the secure default (no `InsecureSkipVerify`). `BaseContext` propagates the app context into in-flight WS handlers so cancellation is clean.
- The in-process `rooms` map (service layer) is the source of truth for relay membership. It is a single-instance design; K8s scale-out would require a shared pub/sub store.
- Read limit is 64 KiB per frame (SDP offers can be multi-KB).
