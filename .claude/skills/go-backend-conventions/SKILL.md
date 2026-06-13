---
name: go-backend-conventions
description: Canonical structure and layered architecture of this monorepo's Go services.
user-invocable: false
---

## Adaptation for this monorepo

This canon is written for a standalone service whose module is `backend`. In THIS repo it applies per
service inside the single module `github.com/pizdagladki/full`:

- Each service lives at `services/<name>/` and has its OWN `internal/` (private to that service — Go
  visibility enforces this). The tree below (`cmd/`, `internal/...`) is rooted at `services/<name>/`.
- Internal imports are `github.com/pizdagladki/full/services/<name>/internal/...`.
- Shared infrastructure (logger, DB) lives in the ROOT `internal/platform/...` and is imported as
  `github.com/pizdagladki/full/internal/platform/...` — do NOT duplicate it inside a service.
- There is no per-service `go.mod`/`go.sum`/`.gitignore`/`README.md` — those are at the repo root. Each
  service keeps only `cmd/`, `internal/`, `Makefile`, `Dockerfile`, `CLAUDE.md`.
- `docker-compose.yml` and `.env.example` live in the repo's `deploy/`.

Everything else below is the source of truth.

---

# Go Backend — project structure (template)

A reusable HTTP backend skeleton in Go. Clean (layered) architecture:
`delivery → service → repository → domain`, plus a separate application-assembly
layer (`app`) and a configuration layer (`config`).

**Base stack of the skeleton:**

- [Fiber v2](https://gofiber.io/) — HTTP server
- [mongo-driver](https://go.mongodb.org/mongo-driver) — MongoDB access
- [zap](https://github.com/uber-go/zap) — structured logging
- [validator/v10](https://github.com/go-playground/validator) — config/DTO validation
- Docker + docker-compose, Makefile, golangci-lint

The module is declared in `go.mod` as `backend` — all internal imports derive from it
(`backend/internal/...`).

---

## Directory tree

```
.
├── cmd/
│   ├── main.go                 # entry point: builds App, runs it with a graceful-shutdown context
│   ├── config.yaml             # local config (gitignored)
│   └── config-example.yaml     # config template to commit
│
├── internal/
│   ├── app/                    # app assembly and lifecycle
│   │   ├── app.go              # struct App: dependency fields + New/Run + init methods
│   │   ├── init_database.go    # DB connection (connect + ping)
│   │   ├── register_http_routes.go  # route and group registration
│   │   ├── run_workers.go      # starts background workers (WaitGroup)
│   │   └── worker_http.go      # HTTP server worker + graceful shutdown
│   │
│   ├── api/
│   │   ├── delivery/           # transport layer (HTTP handlers)
│   │   │   ├── delivery.go         # handler interfaces
│   │   │   ├── <entity>_handler.go # resource handler implementations
│   │   │   └── auth_handler.go     # auth handlers (one per provider)
│   │   │
│   │   ├── service/            # business logic
│   │   │   ├── service.go          # service interfaces
│   │   │   └── <entity>_service.go # resource service implementation
│   │   │
│   │   ├── repository/        # data access
│   │   │   ├── repository.go       # repository interfaces
│   │   │   ├── <entity>_repository.go
│   │   │   └── auth_repository.go
│   │   │
│   │   ├── domain/            # domain models + DTOs (request/response)
│   │   │   ├── <entity>.go
│   │   │   └── user.go
│   │   │
│   │   └── middleware/        # cross-cutting request logic
│   │       └── auth_middleware.go  # session/token check (RequireAuth)
│   │
│   └── config/               # configuration
│       ├── config.go             # config structs + loading (env / yaml)
│       └── validate.go           # config validation via validator
│
├── Dockerfile                 # binary build (multi-stage)
├── docker-compose.yml         # service + dependencies (DB, etc.)
├── Makefile                   # run / build / lint / test / docker-* targets
├── .golangci.yml              # linter config
├── .dockerignore
├── .env.example               # environment variables template
├── .gitignore
├── go.mod
└── README.md
```

---

## Layers and responsibilities

Dependencies point strictly inward: `delivery` knows about `service`, `service` knows about
`repository` and `domain`, `repository` knows about `domain`. There are no reverse dependencies.

| Layer | Directory | Responsible for | Does not do |
|------|---------|-------------|-----------|
| **delivery** | `internal/api/delivery` | request parsing, input validation, response codes, serialization | business rules, DB access |
| **service** | `internal/api/service` | business logic, orchestrating repositories, external integrations | HTTP parsing, direct calls to the DB driver |
| **repository** | `internal/api/repository` | CRUD and DB queries, mapping into domain models | business rules |
| **domain** | `internal/api/domain` | entity models, DTOs, domain types/enums | any I/O logic |
| **middleware** | `internal/api/middleware` | cross-cutting handling (authentication, request context) | — |

**The "interfaces separate from implementations" principle.** In each layer the contracts are
pulled into a single file (`delivery.go`, `service.go`, `repository.go`) and grouped in a
`type ( ... )` block. Implementations sit in neighboring files, one per entity. This simplifies
mocks/tests and makes dependencies explicit.

---

## Application lifecycle (`internal/app`)

`App` is the central struct: it holds all dependencies (logger, validator, config, DB connection,
repositories, services, handlers, middleware) as fields and assembles them in order.

```
main()                       // cmd/main.go
 └─ app.New(serviceName)     // constructor for an empty App
 └─ app.Run(ctx)             // ctx with graceful shutdown on SIGTERM/SIGINT/SIGHUP
      ├─ initLogger()        // zap
      ├─ initValidator()     // validator
      ├─ populateConfig()    // load + validate config
      ├─ initDatabase()      // DB connection + ping
      ├─ initRepositories()  // repository constructors(db)
      ├─ initServices()      // service constructors(repos, cfg, logger)
      ├─ initHandlers()      // handler constructors(services/repos)
      └─ runWorkers(ctx)     // start workers, blocks until they finish
```

Each init step is a separate `App` method (ideally in its own file), which keeps `app.go`
readable: it contains only the struct definition, `New`, `Run`, and the call order.

### Workers (`run_workers.go` + `worker_*.go`)

Background tasks are described by the type `worker func(ctx, *App)`. `runWorkers` launches them as
goroutines under a shared `sync.WaitGroup` and waits for them to finish. The base worker is the
HTTP server (`worker_http.go`): it creates the Fiber app, registers routes, and listens on
`ctx.Done()` for a clean `Shutdown()`. New background processes (cron jobs, queue consumers, etc.)
are added as another `worker` in the slice.

### Route registration (`register_http_routes.go`)

Routes are grouped by version and resource. Protected groups attach the auth middleware to the
whole group:

```go
v1 := app.Group("/v1")

auth := v1.Group("/auth")
auth.Post("/<provider>", a.authHandler.Verify)
auth.Get("/me", a.authMiddleware.RequireAuth(), a.authHandler.GetMe)

entities := v1.Group("/<entities>")
entities.Use(a.authMiddleware.RequireAuth())
entities.Get("/", a.<entity>Handler.List)
entities.Post("/", a.<entity>Handler.Create)
entities.Get("/:id", a.<entity>Handler.Get)
entities.Delete("/:id", a.<entity>Handler.Delete)
```

---

## Configuration (`internal/config`)

The config source is selected at runtime via an environment variable (e.g. `IS_DOCKER`):

- in a container — values are read from environment variables (`loadFromEnv`);
- locally — from the YAML file `cmd/config.yaml` (`loadFromFile`), for which the repo carries
  `config-example.yaml`.

The config is described by nested structs with `yaml:"..."` and `validate:"..."` tags; after
loading, `ValidateConfig` (validator/v10) is called, which fails at startup if required fields are
unset. A getter with a default (`getEnv(key, def)`) sets safe default values.

---

## How to add a new resource (vertical slice)

A single resource touches all layers — order top to bottom:

1. **domain** — `internal/api/domain/<entity>.go`: model + DTOs (request/response) +
   domain types.
2. **repository** — add the interface to `repository.go`, the implementation to
   `<entity>_repository.go`; constructor `New<Entity>Repository(db)`.
3. **service** — interface in `service.go`, implementation in `<entity>_service.go`;
   constructor `New<Entity>Service(repo, cfg, logger)`.
4. **delivery** — interface in `delivery.go`, implementation in `<entity>_handler.go`;
   constructor `New<Entity>Handler(service, logger)`.
5. **app** — add fields to `struct App`, call the constructors in
   `initRepositories/initServices/initHandlers`.
6. **routes** — register the group in `register_http_routes.go`.

---

## Naming conventions

- Files — `snake_case.go`; a layer's interface file is named after the layer
  (`service.go`, `repository.go`, `delivery.go`).
- Implementations — `<entity>_<layer>.go` (`<entity>_service.go`, `<entity>_handler.go`).
- Constructors — `New<Type>(deps...)`, return the layer's interface.
- Private implementation structs — lowercase (`type <entity>Service struct`);
  only the interface is exposed.
- All application code — under `internal/`, so packages can't be imported from outside the module.

---

## Root / infrastructure files

| File | Purpose |
|------|------------|
| `Dockerfile` | multi-stage build of a static binary |
| `docker-compose.yml` | bring up the service together with its dependencies (DB, etc.) |
| `Makefile` | targets `run`, `build`, `test`, `lint`, `docker-up/down/logs` |
| `.golangci.yml` | golangci-lint configuration |
| `.env.example` | environment variables template for container mode |
| `.dockerignore` / `.gitignore` | exclusions for the build and VCS |
| `cmd/config-example.yaml` | YAML config template for local mode |
