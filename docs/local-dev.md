# Local development & deployment runbook

Step-by-step sequences for testing and running the backend and frontend locally, and for deploying.

**Prerequisites:** Go 1.26, Node 20+, Docker, Make, golangci-lint v2. Run `make tools` once to install the
Go dev tools (golangci-lint, mockgen, golang-migrate).

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
4. A dev server (`npm run dev`) is added when the React app lands.

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
