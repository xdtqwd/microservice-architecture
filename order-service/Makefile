.PHONY: help run test test-integration lint migrate-up migrate-down load-test \
        docker-build docker-up docker-down docker-logs coverage coverage-diff

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}'

run: ## Run the service locally
	go run ./cmd

test: ## Run unit tests
	go test -short -race ./...

test-integration: ## Run integration tests (requires DB)
	go test -race -count=1 ./internal/repository/...

GOLANGCI_LINT_VERSION = v2.13.0

install-lint: ## Install golangci-lint
	go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)

lint: ## Run linter
	golangci-lint run

migrate-up: ## Apply migrations with goose
	goose -dir migrations/goose postgres "$(DATABASE_URL)" up

migrate-down: ## Rollback last migration
	goose -dir migrations/goose postgres "$(DATABASE_URL)" down

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

coverage-diff: ## Check diff coverage >= 80%
	go test -coverprofile=coverage.out ./...
	python3 scripts/coverage_diff.py
