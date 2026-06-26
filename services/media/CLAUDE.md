# service: media

Role: HTTP API for win-clip media (upload, download, listing). Stack — the `go-backend-conventions` skill.
Uses: Postgres (pgxpool) + MinIO object storage (`internal/platform/storage`) + Redis (`internal/platform/redis`, session auth for clips). ffmpeg is available in the image for later WebM→MP4 conversion via `os/exec`.

## Commands (from this folder)
- `make help` — list targets. `make run` / `make test` / `make cover` (≥80%) / `make lint` / `make fmt` / `make mocks`.
- `make migrate` — apply golang-migrate migrations.
- `make docker-up` — bring up the service + Postgres + Redis + MinIO locally.

## Responsibility
- Win-clip upload (`POST /v1/clips`) — WebM only, ≤50 MiB, stored in MinIO, metadata in Postgres.
  FIFO keep-last-10 per user: the 11th upload evicts the oldest clip's object and metadata row.
- Clip listing (`GET /v1/clips`) — caller's clips, newest first.
- Clip download (`GET /v1/clips/:id/download`) — returns a pre-signed MinIO URL (15 min TTL by default).
- Session auth via Redis session cookie on all `/v1` routes.
- Liveness: `GET /healthz` → `200 {"status":"ok"}`.
- Migrations: `migrations/0001_clips.up.sql` / `.down.sql` — the `clips` table.

## Config
`cmd/config.yaml` locally / env in Docker (`IS_DOCKER`); template — `config-example.yaml`.

### Environment variables
| Variable | Default | Description |
|---|---|---|
| `HTTP_ADDR` | `:8082` | HTTP listen address |
| `POSTGRES_DSN` | — | Postgres connection string (required) |
| `STORAGE_ENDPOINT` | — | MinIO endpoint (required) |
| `STORAGE_ACCESS_KEY` | — | MinIO access key (required) |
| `STORAGE_SECRET_KEY` | — | MinIO secret key (required) |
| `STORAGE_BUCKET` | — | MinIO bucket name (required) |
| `STORAGE_USE_SSL` | `false` | Enable TLS for MinIO |
| `REDIS_ADDR` | — | Redis address, e.g. `localhost:6379` (required) |
| `REDIS_PASSWORD` | `""` | Redis password |
| `SESSION_COOKIE_NAME` | `session` | Session cookie name |
| `MEDIA_MAX_UPLOAD_BYTES` | `52428800` | Max clip upload size in bytes (50 MiB) |
| `MEDIA_DOWNLOAD_URL_TTL` | `15m` | Pre-signed download URL TTL |

## Gotchas
- Startup pings Postgres, Redis, and verifies MinIO bucket existence; a failed connection aborts startup.
- All `/v1` routes require a valid session cookie (resolved against Redis).
- Shared infra comes from `internal/platform/{logger,postgres,redis,storage}`; never duplicate it inside the service.
- ffmpeg is installed in the Dockerfile's final stage (`debian:12-slim`) via `apt-get`; it is NOT available
  in the distroless base — use the ffmpeg variant Dockerfile when shelling out to ffmpeg.
