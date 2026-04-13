.PHONY: build test vet fmt run tidy db-up db-down db-reset help

APP := overload-party-shop

build: ## Build Docker image
	docker build -t $(APP) .

test: ## Run unit tests (Testcontainers; requires Docker running)
	go test ./... -count=1 -race

vet: ## Run go vet
	go vet ./...

tidy: ## Tidy dependencies
	go mod tidy

fmt: ## Format code
	gofmt -s -w .

db-up: ## Start local Postgres (docker compose)
	docker compose up -d postgres

db-down: ## Stop local Postgres
	docker compose down

db-reset: ## Drop volume and recreate DB
	docker compose down -v
	docker compose up -d postgres

run: db-up ## Run shop server locally against compose Postgres
	go run ./cmd/server

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-12s\033[0m %s\n", $$1, $$2}'

.DEFAULT_GOAL := help
