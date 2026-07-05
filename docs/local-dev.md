# Local development & deployment runbook

Step-by-step sequences for testing and running the backend and frontend locally, and for deploying.

**Prerequisites:** Go 1.26, Node 22+ (24 in CI), Docker, Make, golangci-lint v2. Run `make tools` once to
install the Go dev tools (golangci-lint, mockgen, golang-migrate).

## Full local end-to-end run

On a clean machine, this is the shortest path from nothing to the player path (landing → battle →
results) in a browser:

1. `make -C deploy up` — starts infra only (Postgres, Redis, MinIO).
2. `docker compose -f deploy/docker-compose.yml up -d --build` (run from the repo root) — builds & starts
   all nine app services (auth, signaling, media, store, ratings, reports, matchmaking, koth, health) plus
   the six one-shot DB-migration init steps (auth, ratings, media, store, reports, koth).
3. `cd frontend && npm ci && npm run dev` — starts the Vite dev server.
4. Open the printed Vite URL in a browser and walk the player path: landing → consent → Google login →
   home → mode-select → search → battle → results.

### Port table (host → container, per `deploy/docker-compose.yml`)

| service | host port |
|---|---|
| auth | 8080 |
| signaling | 8081 |
| media | 8082 |
| store | 8083 |
| ratings | 8084 |
| reports | 8085 |
| matchmaking | 8086 |
| koth | 8087 |
| health | 8088 |

### Manual prerequisites for a full walk

- **Backend env:** `cp deploy/env/.env.example deploy/env/.env`, then fill the Google OAuth creds
  (`GOOGLE_OAUTH_CLIENT_ID`, `GOOGLE_OAUTH_CLIENT_SECRET`, `GOOGLE_OAUTH_REDIRECT_URL`) — needed for login;
  Stripe test keys only to exercise purchases.
- **Internal S2S token:** `cp deploy/env/internal.env.example deploy/env/internal.env`, then set a strong
  `INTERNAL_API_TOKEN` (e.g. `openssl rand -hex 32`). Services fail closed on internal routes when it's empty.
- **Frontend env:** `cp frontend/.env.example frontend/.env`, then set `VITE_GOOGLE_CLIENT_ID` and
  `VITE_GOOGLE_REDIRECT_URI` (the browser's Google login link is built from these). The `VITE_API_URL` /
  `VITE_WS_URL` same-origin (empty) defaults route through the Vite dev proxy — leave them empty for the
  local proxied run.

## 1. Backend — local test & run

1. Install dev tools (one-time): `make tools`.
2. Start infra dependencies: `make -C deploy up` (Postgres, Redis, MinIO via docker compose).
3. Apply migrations (services that own tables): `make -C services/<svc> migrate`
   (needs `POSTGRES_DSN`; see `deploy/env/.env.example`).
4. Configure & run a service: copy `services/<svc>/cmd/config-example.yaml` →
   `services/<svc>/cmd/config.yaml` (gitignored), then `make -C services/<svc> run`.
   (Or export `IS_DOCKER=1` + the env vars to use env-mode config.)
5. Smoke-test: `curl localhost:8080/v1/health` → `{"status":"ok"}` (health service).
6. Quality gates — exactly what CI runs:
   - `make -C services/<svc> test` — unit tests (table-driven).
   - `make -C services/<svc> cover` — enforces **≥ 80%** coverage (excludes `cmd/` + generated mocks).
   - `make -C services/<svc> lint` — golangci-lint (strict, `default: all` minus the disable-list).
   - `make -C services/<svc> vet` — `go vet`.
   - `make mocks` — regenerate mocks after changing an interface.
7. Stop infra: `make -C deploy down`.

## 2. Frontend — local test & run

1. `cd frontend && npm ci` (installs from the committed lockfile).
2. Quality gates: `npm run lint` (eslint), `npm run typecheck` (`tsc --noEmit`), `npm run test` (vitest) —
   or `make -C frontend lint typecheck test`.
3. Format: `npm run format` (prettier).
4. `npm run dev` starts the Vite dev server. `VITE_API_URL` / `VITE_WS_URL` default to same-origin (empty)
   and use the Vite dev proxy (added in #154) to reach the backend services, so no per-service URLs or CORS
   config are needed locally. A deployed / non-proxied frontend must set both to the real backend origin(s)
   — see `frontend/.env.example`.

## 3. Deploy (DigitalOcean + Docker compose)

Deploy is **documentation-only for v1** (the finish line is merge into main); there is no CI automation yet.

1. Build service images from the repo root (single `go.mod` must be in context):
   `docker build -f services/<svc>/Dockerfile -t <registry>/<svc>:<tag> .`
   (multi-stage, distroless-static by default; ffmpeg base for services that shell out to ffmpeg).
2. Push images to your registry; pull them on the droplet.
3. On the DigitalOcean droplet, provide a production `deploy/env/.env` (from `.env.example`), then
   `docker compose -f deploy/docker-compose.yml up -d`.
4. Run migrations against production Postgres:
   `migrate -path services/<svc>/migrations -database "$POSTGRES_DSN" up`.
5. Health-check: `curl https://<host>/v1/health` → `200 {"status":"ok"}`.
6. Post-MVP: Kubernetes; coturn (TURN) for WebRTC.

## Notes

- **Windows:** Smart App Control / WDAC may intermittently block freshly built Go **test** binaries
  (`An Application Control policy has blocked this file`). Re-run the command, exclude the Go build temp
  dir, or use WSL. CI (Linux) is unaffected.
