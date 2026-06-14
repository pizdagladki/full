.DEFAULT_GOAL := help

.PHONY: help install lint format typecheck test

help: ## Show this help
	@awk 'BEGIN{FS=":.*## "} /^[a-zA-Z0-9_-]+:.*## /{printf "  \033[36m%-10s\033[0m %s\n",$$1,$$2}' $(MAKEFILE_LIST)

install: ## Install dependencies (npm ci)
	npm ci

lint: ## Lint (eslint)
	npm run lint

format: ## Format (prettier --write)
	npm run format

typecheck: ## Type-check (tsc --noEmit)
	npm run typecheck

test: ## Run tests (vitest)
	npm run test
