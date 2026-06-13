---
name: go-backend-conventions
description: Canonical structure and layered architecture of this monorepo's Go services.
user-invocable: false
---

# Go backend — canon for this monorepo

A reusable HTTP / WebSocket / worker backend skeleton in Go. Clean (layered) architecture:
`delivery → service → repository → domain`, plus an application-assembly layer (`app`), a configuration
layer (`config`), and background workers. Dependencies point strictly inward — `delivery` knows `service`,
`service` knows `repository` + `domain`, `repository` knows `domain`. There are no reverse dependencies.

## Module & layout (this repo)

- ONE module for the whole repo: `github.com/pizdagladki/full` (never `backend`).
- A service lives at `services/<name>/` with its OWN `internal/` (private to that service — Go visibility
  enforces this). Service-private imports: `github.com/pizdagladki/full/services/<name>/internal/...`.
- Shared infrastructure lives in the ROOT `internal/platform/...` and is imported as
  `github.com/pizdagladki/full/internal/platform/...` — services use it, never duplicate it.
- `go.mod` / `go.sum` / `.gitignore` / the root `README.md` are at the repo root; `docker-compose.yml` and
  `.env.example` live in `deploy/`. A service directory contains ONLY: `cmd/`, `internal/`, `migrations/`
  (if it owns tables), `Makefile`, `Dockerfile`, `CLAUDE.md`.

## Stack

- **HTTP**: `net/http` standard library — `http.ServeMux` with Go 1.22+ method+pattern routing
  (`mux.HandleFunc("POST /v1/...", h)`). No third-party web framework.
- **Realtime**: `github.com/gorilla/websocket` — signaling (SDP/ICE exchange), matchmaking, and
  server-side time arbitration.
- **Database**: PostgreSQL via `github.com/jackc/pgx/v5` + `pgxpool`. The repository layer writes SQL by
  hand and maps rows → domain models. Wrap any multi-step, money-touching flow in an explicit transaction.
  Use JSONB columns for flexible fields (e.g. distractor metadata).
- **Migrations**: `golang-migrate` — paired SQL `NNNN_<name>.up.sql` / `.down.sql` in the service's
  `migrations/`; a `make migrate` target applies them. (This is a real concern with Postgres — schema is
  explicit, unlike a schemaless store.)
- **Cache / coordination**: Redis via `github.com/redis/go-redis/v9` — matchmaking queue, hot-data cache
  (e.g. ratings), cooldowns, and sessions.
- **Object storage**: `github.com/minio/minio-go/v7` (MinIO now; the same API targets S3 later).
- **Auth**: Google OAuth via `golang.org/x/oauth2` + `.../oauth2/google`. The session is stored in Redis;
  `auth_middleware.RequireAuth` validates the session; `auth_repository` persists the user in Postgres.
- **Payments**: Stripe via `github.com/stripe/stripe-go` behind a `PaymentProvider` interface in the service
  layer, so an alternative (e.g. an RF) provider drops in without touching purchase logic.
- **Media**: WebM → MP4 conversion shells out to `ffmpeg` via `os/exec` (NOT pure Go).
- **Logging**: `go.uber.org/zap`. **Validation**: `github.com/go-playground/validator/v10` (config + DTOs).
- **Shared `internal/platform/`**: `logger` (zap), `postgres` (pgxpool), `redis` (go-redis),
  `storage` (minio-go). Services import these; never duplicate them.

## Directory tree (rooted at services/<name>/)

```
services/<name>/
├── cmd/
│   ├── main.go                 # entry point: builds App, runs it with a graceful-shutdown context
│   ├── config.yaml             # local config (gitignored)
│   └── config-example.yaml     # config template to commit
│
├── internal/
│   ├── app/                    # app assembly and lifecycle
│   │   ├── app.go              # struct App: dependency fields + New/Run + init methods
│   │   ├── init_postgres.go    # pgxpool connect + ping
│   │   ├── init_redis.go       # go-redis client + ping
│   │   ├── init_storage.go     # minio client
│   │   ├── register_http_routes.go  # ServeMux route registration (HTTP services)
│   │   ├── run_workers.go      # starts background workers (WaitGroup)
│   │   ├── worker_http.go      # net/http server worker + graceful shutdown
│   │   └── worker_ws.go        # gorilla/websocket server worker (realtime services)
│   │
│   ├── api/
│   │   ├── delivery/           # transport layer (HTTP / WS handlers)
│   │   │   ├── delivery.go         # handler interfaces
│   │   │   ├── <entity>_handler.go # resource handler implementations
│   │   │   └── auth_handler.go     # auth handlers (one per provider)
│   │   ├── service/            # business logic
│   │   │   ├── service.go          # service interfaces (incl. PaymentProvider, where used)
│   │   │   └── <entity>_service.go
│   │   ├── repository/         # data access (hand-written SQL via pgx)
│   │   │   ├── repository.go       # repository interfaces
│   │   │   ├── <entity>_repository.go
│   │   │   └── auth_repository.go
│   │   ├── domain/             # domain models + DTOs (request/response) + enums
│   │   │   ├── <entity>.go
│   │   │   └── user.go
│   │   └── middleware/         # cross-cutting request logic
│   │       └── auth_middleware.go  # RequireAuth: validates the Redis session
│   │
│   └── config/                 # configuration
│       ├── config.go               # config structs + loading (env / yaml)
│       └── validate.go             # config validation via validator/v10
│
├── migrations/                 # golang-migrate: NNNN_name.up.sql / .down.sql (if the service owns tables)
├── Dockerfile                  # multi-stage (distroless-static by default; ffmpeg base if it shells out)
├── Makefile                    # run / build / test / lint / vet / migrate / docker-* targets
└── CLAUDE.md                   # service role + which platform deps it uses
```

(No per-service `go.mod`, `.gitignore`, `README.md`, `docker-compose.yml`, or `.env.example` — those live at
the repo root or in `deploy/`.)

## Layers and responsibilities

| Layer | Directory | Responsible for | Does not do |
|------|---------|-------------|-----------|
| **delivery** | `internal/api/delivery` | request parse/validate, status codes, serialization (HTTP responses and WS frames) | business rules, DB access |
| **service** | `internal/api/service` | business logic, orchestrating repositories, external integrations (Stripe via `PaymentProvider`, OAuth, storage) | HTTP parsing, direct pgx calls |
| **repository** | `internal/api/repository` | hand-written SQL via pgx, transactions, mapping rows → domain | business rules |
| **domain** | `internal/api/domain` | entity models, DTOs, domain types/enums | any I/O |
| **middleware** | `internal/api/middleware` | cross-cutting (auth/session, request context) | — |

**Interfaces separate from implementations.** Per layer, contracts go in one file (`delivery.go` /
`service.go` / `repository.go`) inside a `type ( ... )` block; implementations sit in neighbouring
`<entity>_<layer>.go` files. Constructors `New<Type>(deps...)` return the layer's interface; implementation
structs stay lowercase (`type <entity>Service struct`). This keeps dependencies explicit and makes the
repository (and provider) interfaces mockable for unit tests.

## Application lifecycle (`internal/app`)

`App` is the central struct: it holds all dependencies (logger, validator, config, pgxpool, redis client,
minio client, repositories, services, handlers, middleware) as fields and assembles them in order. A service
wires ONLY the dependencies its role needs.

```
main()                       // cmd/main.go
 └─ app.New(serviceName)     // constructor for an empty App
 └─ app.Run(ctx)             // ctx with graceful shutdown on SIGTERM/SIGINT/SIGHUP
      ├─ initLogger()        // zap
      ├─ initValidator()     // validator/v10
      ├─ populateConfig()    // load + validate config
      ├─ initPostgres()      // pgxpool + ping
      ├─ initRedis()         // go-redis client + ping
      ├─ initStorage()       // minio client
      ├─ initRepositories()  // New<Entity>Repository(pool)
      ├─ initServices()      // New<Entity>Service(repo, cfg, logger, ...)
      ├─ initHandlers()      // New<Entity>Handler(service, logger)
      └─ runWorkers(ctx)     // start workers, blocks until they finish
```

Each init step is a separate `App` method (own file) so `app.go` holds only the struct, `New`, `Run`, and
the call order. A service that needs no Redis (or no storage) simply omits that init step and field.

### Workers (`run_workers.go` + `worker_*.go`)

A service exposes HTTP, WebSocket, or runs purely as a background worker depending on its role. Background
tasks are described by `worker func(ctx, *App)`; `runWorkers` launches them as goroutines under a shared
`sync.WaitGroup` and waits for them to finish. Base workers:

- `worker_http.go` — builds the `http.ServeMux`, serves via `http.Server`, and calls `Shutdown()` on
  `ctx.Done()`.
- `worker_ws.go` — the gorilla/websocket realtime server (signaling, matchmaking, server-side time
  arbitration).
- Additional loops (matchmaking, WebM→MP4 conversion via ffmpeg, cron) are added as more `worker`s in the
  slice.

### Route registration (`register_http_routes.go`)

`net/http` ServeMux with method routing; protected routes wrap the handler with the auth middleware:

```go
mux := http.NewServeMux()

mux.HandleFunc("POST /v1/auth/{provider}", a.authHandler.Verify)
mux.Handle("GET /v1/auth/me", a.authMiddleware.RequireAuth(http.HandlerFunc(a.authHandler.GetMe)))

mux.Handle("GET /v1/<entities>", a.authMiddleware.RequireAuth(http.HandlerFunc(a.<entity>Handler.List)))
mux.Handle("POST /v1/<entities>", a.authMiddleware.RequireAuth(http.HandlerFunc(a.<entity>Handler.Create)))
mux.Handle("GET /v1/<entities>/{id}", a.authMiddleware.RequireAuth(http.HandlerFunc(a.<entity>Handler.Get)))
mux.Handle("DELETE /v1/<entities>/{id}", a.authMiddleware.RequireAuth(http.HandlerFunc(a.<entity>Handler.Delete)))
```

## Configuration (`internal/config`)

The config source is selected at runtime via `IS_DOCKER`:

- in a container — values are read from environment variables (`loadFromEnv`);
- locally — from the YAML file `cmd/config.yaml` (`loadFromFile`), for which the repo carries
  `config-example.yaml`.

The config is nested structs with `yaml:"..."` and `validate:"..."` tags; after loading, `ValidateConfig`
(validator/v10) runs and fails at startup if required fields are unset. The config carries (a service
populates only the sections it uses):

- Postgres DSN
- Redis address + password
- MinIO endpoint + access/secret keys + bucket
- Stripe secret key + webhook signing secret
- Google OAuth client id + secret + redirect URL

## How to add a new resource (vertical slice)

Top to bottom — see the `new-resource` skill for the full procedure:

1. **domain** — `domain/<entity>.go`: model + DTOs + enums.
2. **repository** — interface in `repository.go`; impl `<entity>_repository.go` with hand-written SQL
   (transactions where atomicity matters); add a golang-migrate migration for its tables.
   `New<Entity>Repository(pool)`.
3. **service** — interface in `service.go`; impl `<entity>_service.go`; `New<Entity>Service(repo, cfg, logger)`.
4. **delivery** — interface in `delivery.go`; impl `<entity>_handler.go`; `New<Entity>Handler(service, logger)`.
5. **app** — add fields to `struct App`; call constructors in `initRepositories/initServices/initHandlers`.
6. **routes** — register on the ServeMux in `register_http_routes.go`.

## Naming conventions

- Files — `snake_case.go`; a layer's interface file is named after the layer (`service.go`, `repository.go`,
  `delivery.go`).
- Implementations — `<entity>_<layer>.go` (`<entity>_service.go`, `<entity>_handler.go`).
- Constructors — `New<Type>(deps...)`, return the layer's interface.
- Private implementation structs — lowercase (`type <entity>Service struct`); only the interface is exposed.
- All application code — under `internal/`, so packages can't be imported from outside the module.

## Testing

- Repository (and provider, e.g. `PaymentProvider`) interfaces enable mocking → fast unit tests for
  service/handler logic.
- Integration tests that need a real Postgres / Redis / MinIO run them via `testcontainers-go` or
  `dockertest` (optional, guarded so the unit suite stays offline).
