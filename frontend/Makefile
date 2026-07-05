.DEFAULT_GOAL := help

MIN_COVERAGE := 80

.PHONY: help install lint format typecheck test cover

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

cover: ## Run tests with coverage and enforce >=80% (statements)
	npm run cover
	@node -e "\
	const s = require('./coverage/coverage-summary.json').total.statements.pct; \
	console.log('total: (statements) ' + s.toFixed(1) + '%'); \
	if (s < $(MIN_COVERAGE)) { console.error('FAIL: coverage ' + s + '% < $(MIN_COVERAGE)%'); process.exit(1); }"
