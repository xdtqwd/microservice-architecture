.PHONY: help run test test-integration lint migrate-up migrate-down load-test \
        docker-build docker-up docker-down docker-logs coverage coverage-diff

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}'

run: ## Run the service locally
	go run ./cmd

test: ## Run unit tests
	go test -race ./...

test-integration: ## Run integration tests (requires DB)
	DATABASE_URL=postgres://postgres:password@localhost:5436/orders_db go test -race -tags=integration ./...

lint: ## Run linter
	golangci-lint run

migrate-up: ## Apply migrations
	docker exec microservice-architecture-postgres-1 psql -U postgres -d orders_db \
		-f /docker-entrypoint-initdb.d/01_init.sql || true
	docker exec microservice-architecture-postgres-1 psql -U postgres -d orders_db \
		-f /docker-entrypoint-initdb.d/02_schema_constraints.sql || true

migrate-down: ## Drop all tables
	docker exec microservice-architecture-postgres-1 psql -U postgres -d orders_db \
		-c "DROP SCHEMA public CASCADE; CREATE SCHEMA public;"

load-test: ## Run k6 load tests
	k6 run ../load-tests/list_orders.js

docker-build: ## Build docker image
	docker compose -f ../docker-compose.yaml build

docker-up: ## Start all services
	docker compose -f ../docker-compose.yaml up -d

docker-down: ## Stop all services
	docker compose -f ../docker-compose.yaml down

docker-logs: ## Tail logs
	docker compose -f ../docker-compose.yaml logs -f order-service

coverage: ## Generate coverage report
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

coverage-diff: ## Check coverage >= 80%
	go test -coverprofile=coverage.out ./...
	@go tool cover -func=coverage.out | grep total | awk '{print $$3}' | \
		tr -d '%' | awk '{if ($$1 < 80) {print "Coverage " $$1 "% < 80%"; exit 1} else {print "Coverage " $$1 "% OK"}}'
