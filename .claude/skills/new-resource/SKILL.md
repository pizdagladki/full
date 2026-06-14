---
name: new-resource
description: Add a resource as a vertical slice (domain→repository→service→delivery→app→routes) in a service.
---
Add a resource to `services/<svc>/` strictly per `go-backend-conventions` (read it first). A resource touches
every layer — implement top to bottom. Interfaces live in the per-layer file (`repository.go` / `service.go` /
`delivery.go`) inside a `type ( ... )` block; implementations sit in `<entity>_<layer>.go`. Constructors
`New<Entity><Layer>(deps...)` return the interface; the implementation struct stays lowercase.

## Order (vertical slice)
1. **domain** — `internal/api/domain/<entity>.go`: model + request/response DTOs + domain types/enums. No I/O.
2. **repository** — add the interface to `repository.go`; implement in `<entity>_repository.go`;
   constructor `New<Entity>Repository(pool)`.
   - PostgreSQL via pgx: write the SQL by hand and map rows → domain models. No business rules here.
   - Wrap multi-step writes in an explicit transaction (`pool.Begin` → `tx.Commit` / `tx.Rollback`) wherever
     atomicity matters — anything money-touching. Use JSONB columns for flexible fields.
   - Add a golang-migrate migration for the resource's tables: paired `migrations/NNNN_<name>.up.sql` and
     `migrations/NNNN_<name>.down.sql`.
   - Add `//go:generate mockgen -source=repository.go -destination=mocks/repository_mock.go -package=mocks`
     on `repository.go`; run `make mocks` and use the mock to unit-test the service layer.
3. **service** — interface in `service.go`; implement in `<entity>_service.go`;
   constructor `New<Entity>Service(repo, cfg, logger)`. Business logic, orchestration, external integrations
   (Stripe via the `PaymentProvider` interface, OAuth, storage). No HTTP parsing, no raw pgx calls.
4. **delivery** — interface in `delivery.go`; implement in `<entity>_handler.go`;
   constructor `New<Entity>Handler(service, logger)`. Request parse/validate, status codes, serialization only.
5. **app** — add fields to `struct App`; call the constructors in `initRepositories` / `initServices` /
   `initHandlers`.
6. **routes** — register on the `*echo.Echo` in `register_http_routes.go`; wrap protected routes with
   `authMiddleware.RequireAuth`.

## Tests & checks
- Write **table-driven** tests (`t.Run` over a slice of cases) for EVERY acceptance criterion (handler- and
  service-level). Generate a mock of the repository/service interface with mockgen (`//go:generate ...` on
  the interface file + `make mocks`) and use it so the unit suite stays offline.
- Keep coverage **≥ 80%**: `make -C services/<svc> cover` (excludes `cmd/` + mocks); `... lint` and `... test`
  green too. Show the output before opening the PR.
