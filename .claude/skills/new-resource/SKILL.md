---
name: new-resource
description: Add a resource as a vertical slice (domain→repository→service→delivery→app→routes) in a service.
---
Add a resource to an existing service `services/<svc>/` strictly per `go-backend-conventions` (read it first).
A resource touches every layer — implement top to bottom. Interfaces live in the per-layer file
(`repository.go` / `service.go` / `delivery.go`) inside a `type ( ... )` block; implementations sit in
`<entity>_<layer>.go`. Constructors `New<Entity><Layer>(deps...)` return the interface; the implementation
struct stays lowercase (`type <entity>Service struct`).

## Order (vertical slice)
1. **domain** — `internal/api/domain/<entity>.go`: model + request/response DTOs + domain types/enums. No I/O.
2. **repository** — add the interface to `repository.go`; implement in `<entity>_repository.go`;
   constructor `New<Entity>Repository(db)`. CRUD + mapping into domain models only — no business rules.
3. **service** — add the interface to `service.go`; implement in `<entity>_service.go`;
   constructor `New<Entity>Service(repo, cfg, logger)`. Business logic, orchestration; no HTTP parsing,
   no raw driver calls.
4. **delivery** — add the interface to `delivery.go`; implement in `<entity>_handler.go`;
   constructor `New<Entity>Handler(service, logger)`. Request parse/validate, status codes, serialization only.
5. **app** — add fields to `struct App`; call the constructors in `initRepositories` / `initServices` /
   `initHandlers`.
6. **routes** — register the group in `register_http_routes.go`; attach `a.authMiddleware.RequireAuth()`
   to protected groups.

## Tests & checks
- Write tests for EVERY acceptance criterion of the issue (handler-level, plus service-level where logic warrants).
- `make -C services/<svc> test` and `... lint` green; show the output before opening the PR.
