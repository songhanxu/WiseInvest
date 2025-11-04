.PHONY: help build run test clean deps lint db-init db-reset dev

help: ## Display this help screen
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'

build: ## Build the application
	go build -o bin/server cmd/server/main.go

run: ## Run the application
	go run cmd/server/main.go

test: ## Run tests
	go test -v -race -coverprofile=coverage.out ./...

test-coverage: test ## Run tests with coverage report
	go tool cover -html=coverage.out

clean: ## Clean build artifacts
	rm -rf bin/
	rm -f coverage.out

deps: ## Download dependencies
	go mod download
	go mod tidy

lint: ## Run linter (requires golangci-lint)
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run; \
	else \
		echo "golangci-lint not installed. Install with: brew install golangci-lint"; \
	fi

db-init: ## Initialize database
	@echo "Initializing database..."
	@psql postgres -c "CREATE USER wiseinvest WITH PASSWORD 'wiseinvest123';" 2>/dev/null || true
	@psql postgres -c "CREATE DATABASE wiseinvest OWNER wiseinvest;" 2>/dev/null || true
	@psql postgres -c "GRANT ALL PRIVILEGES ON DATABASE wiseinvest TO wiseinvest;" 2>/dev/null || true
	@echo "Database initialized successfully"

db-reset: ## Reset database (WARNING: deletes all data)
	@echo "Resetting database..."
	@psql postgres -c "DROP DATABASE IF EXISTS wiseinvest;"
	@psql postgres -c "CREATE DATABASE wiseinvest OWNER wiseinvest;"
	@echo "Database reset successfully"

db-connect: ## Connect to database
	psql -h localhost -U wiseinvest -d wiseinvest

services-start: ## Start PostgreSQL and Redis
	@echo "Starting services..."
	@brew services start postgresql@15
	@brew services start redis
	@echo "Services started"

services-stop: ## Stop PostgreSQL and Redis
	@echo "Stopping services..."
	@brew services stop postgresql@15
	@brew services stop redis
	@echo "Services stopped"

services-status: ## Check services status
	@brew services list | grep -E "postgresql|redis"

dev: ## Run in development mode with hot reload (requires air)
	@if command -v air >/dev/null 2>&1; then \
		air; \
	else \
		echo "air not installed. Install with: go install github.com/cosmtrek/air@latest"; \
		echo "Running without hot reload..."; \
		go run cmd/server/main.go; \
	fi

install-tools: ## Install development tools
	@echo "Installing development tools..."
	@go install github.com/cosmtrek/air@latest
	@brew install golangci-lint
	@echo "Tools installed successfully"

.DEFAULT_GOAL := help
