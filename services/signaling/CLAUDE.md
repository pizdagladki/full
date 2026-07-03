# service: signaling

Role: WebSocket realtime signaling. Stack — the `go-backend-conventions` skill. Uses: Redis (go-redis).

## Commands (from this folder)
- `make help` — list targets. `make run` / `make test` / `make cover` (≥80%) / `make lint` / `make fmt` / `make mocks`.
- `make docker-up` — bring up the service + Redis locally.

## Responsibility
- WebRTC SDP/ICE relay between two room peers over WebSocket (`coder/websocket`).
- Invite-a-friend private rooms: an authenticated player creates a room (`create_room`) and gets a fresh
  `room_id` + a short shareable invite `code`; a second player joins the SAME room by that code
  (`join_room`), bypassing matchmaking. Private rooms are always **unranked** — no ratings call is ever
  made on their outcome.
- Liveness: `GET /healthz` → `200 {"status":"ok"}`.
- Config: `cmd/config.yaml` locally / env in Docker (`IS_DOCKER`, `HTTP_ADDR`, `REDIS_ADDR`, `REDIS_PASSWORD`,
  `SIG_SESSION_COOKIE`, `SIG_ROOM_TTL`, `SIG_ROOM_CODE_TTL`); template — `config-example.yaml`.
- New resource — via the `new-resource` skill.

## Message protocol (`GET /ws`)

Authentication: the client must present a valid session cookie (`session` by default) set by the auth service.
The session maps to a `userID` stored in Redis under `session:<value>`.

All messages are JSON text frames.

### Client → Server

| Type        | Required fields            | Description                                                       |
|-------------|-----------------------------|--------------------------------------------------------------------|
| join        | `room_id`                  | Join or re-join a signaling room (max 2 members).                 |
| sdp         | `room_id`, `sdp` (+ any)   | Relay an SDP offer or answer verbatim to the peer.                 |
| ice         | `room_id`, `candidate`     | Relay an ICE candidate verbatim to the peer.                       |
| create_room | —                          | Create a fresh private (unranked) room; get back `room_id`+`code`. |
| join_room   | `code`                     | Join a private room by its invite code (bypasses matchmaking).     |

### Server → Client

| Type         | Fields              | Description                                                  |
|--------------|---------------------|----------------------------------------------------------------|
| error        | `error`             | Sent on validation failure, full room, invalid code, or bad type. |
| peer_left    | —                   | Sent when the other peer disconnects.                          |
| room_created | `room_id`, `code`   | Reply to `create_room`: the fresh room and its shareable code.  |
| room_joined  | `room_id`           | Reply to `join_room`: the room the caller was admitted into.    |

### Room rules
- A room holds at most **2** members (`room:<roomID>:members` Redis SET, TTL from config).
- A third `join` gets `{"type":"error","error":"room is full"}` and the connection is closed.
- `sdp`/`ice` are forwarded **verbatim** (raw bytes) to the other member only — never echoed, never to other rooms.
- A sender not in the room (in-process hub) gets `{"type":"error","error":"not a member of this room"}`.
- On disconnect the remaining peer receives `{"type":"peer_left"}` and the room is deleted from Redis.

### Private rooms (invite-a-friend)
- `create_room` generates a fresh `room_id`, registers the caller as its first member (always
  **unranked** — mode is never negotiable via this path), mints a short invite `code`
  (`roomcode:<code>` → `roomID` in Redis, TTL = `signaling.room_code_ttl`), and replies `room_created`.
- `join_room` resolves the code to its `room_id` and joins the caller as the second member of the SAME
  room used by the SDP/ICE relay above — bypassing matchmaking entirely. An unknown/expired code gets
  `{"type":"error","error":"invalid or expired code"}` and the connection stays open (the client may
  retry); a full private room gets `{"type":"error","error":"room is full"}` and the connection is closed.
- Because a private room is always unranked, its outcome (decided blink or forfeit) NEVER triggers a
  ratings `ApplyResult` call.
- If the creator disconnects before a second peer joins, `Leave` removes both the in-process room and the
  invite code (`roomCodeRepo.RemoveCode`); the code TTL is the backstop if that call fails.

## Redis keys
- `session:<sessionID>` → int64 userID (managed by the auth service)
- `room:<roomID>:members` → Redis SET of base-10 userID strings; TTL = `signaling.room_ttl` (default 30m)
- `roomcode:<code>` → string roomID; TTL = `signaling.room_code_ttl` (default 15m)

## Gotchas
- Startup pings Redis (via `internal/platform/redis`); a failed ping aborts startup. A local run therefore needs Redis reachable — `make docker-up` brings it up.
- Shared infra comes from `internal/platform/{logger,redis}`; never duplicate it inside the service.
- No Echo dependency — the HTTP surface is a plain `net/http` server in `internal/app/worker_ws.go`.
- `websocket.Accept` uses the secure default (no `InsecureSkipVerify`). `BaseContext` propagates the app context into in-flight WS handlers so cancellation is clean.
- The in-process `rooms` map (service layer) is the source of truth for relay membership. It is a single-instance design; K8s scale-out would require a shared pub/sub store.
- Read limit is 64 KiB per frame (SDP offers can be multi-KB).
