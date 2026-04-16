.PHONY: build test test-integration vet fmt run tidy db-up db-down db-reset help

APP := overload-party-shop

build: ## Build Docker image
	docker build -t $(APP) .

test: ## Run unit tests (Testcontainers; requires Docker running)
	go test ./... -count=1 -race

test-integration: ## Run unit + integration tests (Pub/Sub emulator container; slower)
	go test -tags=integration ./... -count=1 -race

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

run: db-up ## Run shop server locally against compose Postgres (local env 込み)
	PORT=9006 \
	DATABASE_CONN="host=localhost port=5432 dbname=shop user=shop password=shop sslmode=disable" \
	GOOGLE_CLOUD_PROJECT=shop-local \
	FACTION_SELECTED_TOPIC=faction-selected \
	PREMIUM_UPDATED_TOPIC=premium-updated \
	IAP_MODE=local \
	OUTBOX_POLL_INTERVAL=1s \
	OUTBOX_BATCH_SIZE=100 \
	OUTBOX_FAILURE_THRESHOLD=5 \
	OUTBOX_VISIBILITY_TIMEOUT=30s \
	PUBSUB_EMULATOR_HOST=localhost:8085 \
	FIRESTORE_EMULATOR_HOST=localhost:9041 \
	go run ./cmd/server

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-12s\033[0m %s\n", $$1, $$2}'

.DEFAULT_GOAL := help
