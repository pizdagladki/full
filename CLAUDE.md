# full — monorepo

Go microservices (`services/`), frontend (`frontend/`), shared Go code (`internal/`).

## Where things live
- `services/<name>/` — a microservice, layered architecture. Details — in its CLAUDE.md.
- `internal/platform/` — shared code for all services (logger, DB). Do NOT duplicate it inside services.
- `frontend/` — frontend, its own ecosystem.
- The backend architecture canon — the `go-backend-conventions` skill (apply it when working on services).

## Commands (from the root)
- `make -C services/<name> test` — tests for a single service. Prefer this over `make test` for everything.
- `make lint` / `make build` — across the whole repo.

## Workflow — MANDATORY
- One task = one issue = one branch. Do NOT touch files of someone else's active issue.
- Before a PR: `make lint` and the service's tests green, attach the output.
- PR with `Closes #<N>`. Do not push to `main` directly — PRs only.
- NEVER edit `.github/` — that's the human's zone.
