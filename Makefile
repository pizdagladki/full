.DEFAULT_GOAL := help

SERVICES := $(wildcard services/*)
GOLANGCI_VERSION := v2.11.4

.PHONY: help lint test build typecheck cover mocks fmt tools

help: ## Show available targets
	@awk 'BEGIN{FS=":.*## "} /^[a-zA-Z0-9_-]+:.*## /{printf "  \033[36m%-12s\033[0m %s\n",$$1,$$2}' $(MAKEFILE_LIST)

lint: ## Lint every service (golangci-lint)
	@for s in $(SERVICES); do $(MAKE) -C $$s lint || exit 1; done

test: ## Test every service
	@for s in $(SERVICES); do $(MAKE) -C $$s test || exit 1; done

cover: ## Enforce >=80% coverage in every service
	@for s in $(SERVICES); do $(MAKE) -C $$s cover || exit 1; done

build: ## Build every service
	@for s in $(SERVICES); do $(MAKE) -C $$s build || exit 1; done

typecheck: ## go vet every service
	@for s in $(SERVICES); do $(MAKE) -C $$s vet || exit 1; done

mocks: ## Regenerate mocks in every service
	@for s in $(SERVICES); do $(MAKE) -C $$s mocks || exit 1; done

fmt: ## Format every service (gofmt + goimports)
	@for s in $(SERVICES); do $(MAKE) -C $$s fmt || exit 1; done

tools: ## Install dev tools (golangci-lint, mockgen, migrate)
	go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_VERSION)
	go install go.uber.org/mock/mockgen@latest
	go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest
