.DEFAULT_GOAL := help

COMPOSE := docker compose -f docker-compose.yml

.PHONY: help up down logs ps

help: ## Show this help
	@awk 'BEGIN{FS=":.*## "} /^[a-zA-Z0-9_-]+:.*## /{printf "  \033[36m%-8s\033[0m %s\n",$$1,$$2}' $(MAKEFILE_LIST)

up: ## Start infra (postgres, redis, minio)
	$(COMPOSE) up -d postgres redis minio

down: ## Stop and remove infra
	$(COMPOSE) down

logs: ## Tail infra logs
	$(COMPOSE) logs -f

ps: ## List running containers
	$(COMPOSE) ps
