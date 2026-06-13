# full — monorepo

Go microservices (`services/`), a React frontend (`frontend/`), shared Go code (`internal/`).

## Where things live
- `services/<name>/` — a Go microservice (HTTP / WebSocket / worker), layered architecture. Details — its CLAUDE.md.
- `internal/platform/` — shared infra for all services: `logger` (zap), `postgres` (pgxpool), `redis` (go-redis), `storage` (minio-go). Do NOT duplicate it inside services.
- `frontend/` — frontend, its own ecosystem.
- `deploy/` — docker-compose + env templates. `docs/` — architecture, ADRs, specs.
- The backend architecture canon — the `go-backend-conventions` skill (apply it when working on services).

## Stack
- **Backend (Go):** `net/http` (stdlib ServeMux, 1.22+ method routing); `gorilla/websocket` (signaling / matchmaking / server-side time arbitration); PostgreSQL via `pgx v5` + `pgxpool` (hand-written SQL, transactions for money flows, JSONB); `golang-migrate` (per-service `migrations/`); Redis via `go-redis v9` (queue / cache / cooldowns / sessions); MinIO via `minio-go v7` (S3-compatible); Google OAuth (`golang.org/x/oauth2`, session in Redis); Stripe via `stripe-go` behind a `PaymentProvider` interface (RF provider later); WebM→MP4 via `ffmpeg` (`os/exec`); `zap` + `validator/v10`.
- **Frontend (React):** MediaPipe FaceLandmarker / EAR; WebRTC P2P + STUN (TURN later); canvas + MediaRecorder (WebM); Canvas/WebGL edit templates. Keep canvas / CV / WebRTC in isolated components behind refs.
- **Infra:** DigitalOcean + Docker compose (Go + Postgres + Redis + MinIO; coturn later); Kubernetes post-MVP.
- **External:** Stripe (RF later), AdSense, AdMob/Unity, a Telegram bug-report bot, Google OAuth.

## Commands (from the root)
- `make -C services/<name> test` — tests for a single service. Prefer this over `make test` for everything.
- `make lint` / `make build` — across the whole repo.

## Workflow — MANDATORY
- One task = one issue = one branch. Do NOT touch files of someone else's active issue.
- Before a PR: `make lint` and the service's tests green, attach the output.
- PR with `Closes #<N>`. Do not push to `main` directly — PRs only.
- NEVER edit `.github/` — that's the human's zone.
