.DEFAULT_GOAL := help

GOLANGCI_VERSION := v2.11.4
MIN_COVERAGE := 80

# On Windows, Application Control policies may block newly-compiled test
# binaries when multiple packages are built in parallel (the policy cache is
# not always warmed fast enough). Limit package-level parallelism to 1 and
# redirect the build temp dir to a workspace-local path that is always
# allowed by the policy.
export GOTMPDIR := $(CURDIR)/../../.gotmp
GO_TEST_FLAGS := -p 1

.PHONY: help run build test cover vet lint fmt generate mocks migrate tools docker-up docker-down

help: ## Show this help
	@awk 'BEGIN{FS=":.*## "} /^[a-zA-Z0-9_-]+:.*## /{printf "  \033[36m%-12s\033[0m %s\n",$$1,$$2}' $(MAKEFILE_LIST)

run: ## Run the service locally
	go run ./cmd

build: ## Build the service binary
	go build -o bin/app ./cmd

test: ## Run tests
	go test $(GO_TEST_FLAGS) ./... -count=1

cover: ## Run tests and enforce >=80% coverage (excludes cmd + generated mocks)
	@pkgs=$$(go list ./... | grep -v -e '/cmd$$' -e '/mocks$$'); \
	go test $(GO_TEST_FLAGS) $$pkgs -coverpkg=$$(echo $$pkgs | tr ' ' ,) -covermode=atomic -coverprofile=coverage.out -count=1; \
	total=$$(go tool cover -func=coverage.out | awk '/^total:/{gsub(/%/,"",$$3); print $$3}'); \
	rm -f coverage.out; \
	echo "total coverage: $$total% (min $(MIN_COVERAGE)%)"; \
	awk "BEGIN{exit !($$total+0 >= $(MIN_COVERAGE))}" || { echo "FAIL: coverage $$total% < $(MIN_COVERAGE)%"; exit 1; }

vet: ## Run go vet
	go vet ./...

lint: ## Run golangci-lint
	golangci-lint run

fmt: ## Format (gofmt + goimports via golangci-lint v2)
	golangci-lint fmt

generate: ## Run go generate (regenerate mocks)
	go generate ./...

mocks: generate ## Regenerate mocks

migrate: ## Apply golang-migrate migrations
	migrate -path migrations -database "$(POSTGRES_DSN)" up

tools: ## Install dev tools (golangci-lint, mockgen, migrate)
	go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_VERSION)
	go install go.uber.org/mock/mockgen@latest
	go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest

docker-up: ## Bring up the service + Postgres + Redis + MinIO
	docker compose -f ../../deploy/docker-compose.yml up -d $(notdir $(CURDIR)) postgres redis minio

docker-down: ## Stop the local stack
	docker compose -f ../../deploy/docker-compose.yml down
