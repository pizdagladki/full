# service: media

Role: HTTP API for win-clip media (upload, download, WebM→MP4 conversion). Stack — the `go-backend-conventions` skill.
Uses: Postgres (pgxpool) + MinIO object storage (`internal/platform/storage`). ffmpeg is available in the image for later WebM→MP4 conversion via `os/exec`. No Redis.

## Commands (from this folder)
- `make help` — list targets. `make run` / `make test` / `make cover` (≥80%) / `make lint` / `make fmt` / `make mocks`.
- `make migrate` — apply golang-migrate migrations (added once the service owns Postgres tables).
- `make docker-up` — bring up the service + Postgres + MinIO locally.

## Responsibility
- Win-clip media upload/download via MinIO; WebM→MP4 transcoding via ffmpeg (downstream).
- Liveness: `GET /healthz` → `200 {"status":"ok"}`.
- Config: `cmd/config.yaml` locally / env in Docker (`IS_DOCKER`, `HTTP_ADDR`, `POSTGRES_DSN`,
  `STORAGE_ENDPOINT`, `STORAGE_ACCESS_KEY`, `STORAGE_SECRET_KEY`, `STORAGE_BUCKET`, `STORAGE_USE_SSL`);
  template — `config-example.yaml`.
- New resource — via the `new-resource` skill.

## Gotchas
- Startup pings Postgres (via `internal/platform/postgres`) and verifies MinIO bucket existence (via
  `internal/platform/storage`); a failed connection aborts startup. A local run therefore needs both
  reachable — `make docker-up` brings them up.
- Shared infra comes from `internal/platform/{logger,postgres,storage}`; never duplicate it inside the service.
- ffmpeg is installed in the Dockerfile's final stage (`debian:12-slim`) via `apt-get`; it is NOT available
  in the distroless base — use the ffmpeg variant Dockerfile when shelling out to ffmpeg.
