.DEFAULT_GOAL := help

GOLANGCI_VERSION := v2.11.4
MIN_COVERAGE := 80

.PHONY: help run build test cover vet lint fmt generate mocks tools

help: ## Show this help
	@awk 'BEGIN{FS=":.*## "} /^[a-zA-Z0-9_-]+:.*## /{printf "  \033[36m%-10s\033[0m %s\n",$$1,$$2}' $(MAKEFILE_LIST)

run: ## Run the service locally
	go run ./cmd

build: ## Build the service binary
	go build -o bin/app ./cmd

test: ## Run tests
	go test ./... -count=1

cover: ## Run tests and enforce >=80% coverage (excludes cmd + generated mocks)
	@pkgs=$$(go list ./... | grep -v -e '/cmd$$' -e '/mocks$$'); \
	go test $$pkgs -coverpkg=$$(echo $$pkgs | tr ' ' ,) -covermode=atomic -coverprofile=coverage.out -count=1; \
	total=$$(go tool cover -func=coverage.out | awk '/^total:/{gsub(/%/,"",$$3); print $$3}'); \
	rm -f coverage.out; \
	echo "total coverage: $$total% (min $(MIN_COVERAGE)%)"; \
	awk "BEGIN{exit !($$total+0 >= $(MIN_COVERAGE))}" || { echo "FAIL: coverage $$total% < $(MIN_COVERAGE)%"; exit 1; }

vet: ## Run go vet
	go vet ./...

lint: ## Run golangci-lint
	golangci-lint run

fmt: ## Format code (gofmt + goimports via golangci-lint v2)
	golangci-lint fmt

generate: ## Run go generate (regenerate mocks)
	go generate ./...

mocks: generate ## Regenerate mocks

tools: ## Install dev tools (golangci-lint, mockgen, migrate)
	go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_VERSION)
	go install go.uber.org/mock/mockgen@latest
	go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest
