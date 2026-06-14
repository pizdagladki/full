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
- `go.mod` / `go.sum` / `.gitignore` / the root `README.md` are at the repo root; `docker-compose.yml` lives
  in `deploy/` and `.env.example` in `deploy/env/`. A service directory contains ONLY: `cmd/`, `internal/`,
  `migrations/` (if it owns tables), `Makefile`, `Dockerfile`, `CLAUDE.md`.

## Stack

- **HTTP**: `github.com/labstack/echo/v4` (Echo) — router, middleware, request binding and validation.
  Handlers are `func(c echo.Context) error`; routes via `e.POST("/v1/...", h)`. Wire `validator/v10` as
  Echo's `e.Validator`. No other web framework.
- **Realtime**: `github.com/coder/websocket` — signaling (SDP/ICE exchange), matchmaking, and
  server-side time arbitration. Context-native (`Accept`/`Read`/`Write` take a `context.Context`);
  concurrent writes are safe, so no per-connection write mutex.
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
│   │   ├── register_http_routes.go  # Echo route registration (HTTP services)
│   │   ├── run_workers.go      # starts background workers (WaitGroup)
│   │   ├── worker_http.go      # Echo server worker (e.Start) + graceful shutdown (e.Shutdown)
│   │   └── worker_ws.go        # coder/websocket server worker (realtime services)
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
the repo root or under `deploy/`, with `.env.example` in `deploy/env/`.)

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

- `worker_http.go` — builds the `*echo.Echo`, serves via `e.Start(addr)`, and calls `e.Shutdown(ctx)` on
  `ctx.Done()` (treat the returned `http.ErrServerClosed` as a clean stop).
- `worker_ws.go` — the coder/websocket realtime server (signaling, matchmaking, server-side time
  arbitration).
- Additional loops (matchmaking, WebM→MP4 conversion via ffmpeg, cron) are added as more `worker`s in the
  slice.

### Route registration (`register_http_routes.go`)

Echo router; protected routes share the auth middleware via a group. Path params use `:name` (Echo), not `{name}`:

```go
e := echo.New()
e.Validator = a.validator // validator/v10 wrapper

e.POST("/v1/auth/:provider", a.authHandler.Verify)

// Protected routes share the auth middleware via a group.
v1 := e.Group("/v1", a.authMiddleware.RequireAuth)
v1.GET("/auth/me", a.authHandler.GetMe)

v1.GET("/<entities>", a.<entity>Handler.List)
v1.POST("/<entities>", a.<entity>Handler.Create)
v1.GET("/<entities>/:id", a.<entity>Handler.Get)
v1.DELETE("/<entities>/:id", a.<entity>Handler.Delete)
```

Handlers implement `func(c echo.Context) error`; `RequireAuth` is an `echo.MiddlewareFunc`
(`func(echo.HandlerFunc) echo.HandlerFunc`) that validates the Redis session.

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
6. **routes** — register on the `*echo.Echo` in `register_http_routes.go`.

## Naming conventions

- Files — `snake_case.go`; a layer's interface file is named after the layer (`service.go`, `repository.go`,
  `delivery.go`).
- Implementations — `<entity>_<layer>.go` (`<entity>_service.go`, `<entity>_handler.go`).
- Constructors — `New<Type>(deps...)`, return the layer's interface.
- Private implementation structs — lowercase (`type <entity>Service struct`); only the interface is exposed.
- All application code — under `internal/`, so packages can't be imported from outside the module.

## Testing & quality gates

- **Table-driven tests** (`t.Run` over a slice of cases) are the default style.
- **Coverage ≥ 80%** per service, enforced by `make cover` (excludes `cmd/` and generated mocks); CI fails
  below the threshold.
- **Mocks** via mockgen (`go.uber.org/mock`): put `//go:generate mockgen -source=<layer>.go
  -destination=mocks/<layer>_mock.go -package=mocks` on the interface file (`repository.go`, `service.go`),
  run `make mocks`, and use the generated mock to unit-test the layer above (e.g. mock the repository when
  testing a service). Generated files are excluded from the linter.
- Integration tests that need a real Postgres / Redis / MinIO run them via `testcontainers-go` or
  `dockertest` (optional, guarded so the unit suite stays offline).
- **Lint**: golangci-lint v2, `default: all` minus the project disable-list (`.golangci.yml`). `make lint`
  (and `make fmt` for gofmt+goimports). `make help` lists targets; `make tools` installs the dev tools.
- Local test / run / deploy sequences: see `docs/local-dev.md`.
