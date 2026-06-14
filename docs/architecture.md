# Architecture

System overview of the `full` monorepo. This is NOT the backend canon — the canonical Go-service
structure lives in the `go-backend-conventions` skill.

## Layout
- `services/<name>/` — independent Go microservices, one shared module (`github.com/pizdagladki/full`).
  Service boundaries are enforced by the double `internal/`: `services/<name>/internal/` is private to
  that service (Go visibility), while the root `internal/` is shared across services but unreachable
  from outside the repo.
- `internal/platform/` — shared infrastructure reused by all services: `logger` (zap) exists today;
  `postgres` (pgxpool), `redis` (go-redis), `storage` (minio-go) arrive with the first service that needs
  them (the `go-backend-conventions` skill defines the target set).
- `frontend/` — frontend application, its own ecosystem.
- `deploy/` — docker-compose and env templates for local bring-up.

## Service architecture
Layered: `delivery → service → repository → domain`, assembled in `internal/app`, configured in
`internal/config`. Dependencies point strictly inward. See the `go-backend-conventions` skill.

## How work flows
Coordination runs entirely through GitHub: Issues (queue), PRs (changes), labels (state), branch
protection (the gate). One task = one issue = one branch = one zone.
