.PHONY: build run dev test lint tidy clean up down migrate-up migrate-down import-olist seed-demo

# Build variables
BINARY_NAME=server
BUILD_DIR=bin

# Go variables
GOCMD=go
GOBUILD=$(GOCMD) build
GORUN=$(GOCMD) run
GOTEST=$(GOCMD) test
GOMOD=$(GOCMD) mod

# Build the server
build:
	@echo "Building server..."
	@mkdir -p $(BUILD_DIR)
	@$(GOBUILD) -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/server
	@echo "✓ Server built: $(BUILD_DIR)/$(BINARY_NAME)"

# Run the server
run: build
	@./$(BUILD_DIR)/$(BINARY_NAME)

# Run with hot reload (requires air: go install github.com/air-verse/air@latest)
dev:
	@air -c .air.toml || $(GORUN) ./cmd/server

# Run tests
test:
	@$(GOTEST) -v ./...

# Run tests with coverage
test-coverage:
	@$(GOTEST) -v -coverprofile=coverage.out ./...
	@$(GOCMD) tool cover -html=coverage.out -o coverage.html
	@echo "✓ Coverage report: coverage.html"

# Run linter (requires golangci-lint)
lint:
	@golangci-lint run ./...

# Tidy dependencies
tidy:
	@$(GOMOD) tidy

# Clean build artifacts
clean:
	@rm -rf $(BUILD_DIR)
	@rm -f coverage.out coverage.html
	@echo "✓ Cleaned"

# Docker: Postgres + Redis + migrations + backend
up:
	@docker compose up -d --build --wait

down:
	@docker compose down

# Apply all pending migrations (also run automatically by `make up`)
migrate-up:
	@docker compose run --rm migrate

# Roll back the last migration
migrate-down:
	@docker compose run --rm migrate 'exec migrate -path /migrations -database "$$DATABASE_URL" down 1'

# psql in a one-off container of the compose postgres image (same network and credentials)
OLIST_DIR ?= ../data/olist
PSQL_RUN = docker compose run --rm --no-deps -T -v $(CURDIR)/scripts:/scripts:ro
PSQL = sh -c 'PGPASSWORD="$$POSTGRES_PASSWORD" exec psql -h postgres -U "$$POSTGRES_USER" -d "$$POSTGRES_DB" -v ON_ERROR_STOP=1 -f "$$0"'

# Import the 9 Olist CSVs into olist_raw and sfd (once, after migrate-up)
import-olist:
	@$(PSQL_RUN) -v $(abspath $(OLIST_DIR)):/data/olist:ro postgres $(PSQL) /scripts/import_olist.sql

# Create demo accounts seller_demo / buyer_demo (password demo1234), after import-olist
seed-demo:
	@$(PSQL_RUN) postgres $(PSQL) /scripts/seed_demo.sql

# Generate swagger docs (requires swag)
swagger:
	@swag init -g cmd/server/main.go -o docs

# Show help
help:
	@echo "Available targets:"
	@echo "  build          - Build the server binary"
	@echo "  run            - Build and run the server"
	@echo "  dev            - Run with hot reload (requires air)"
	@echo "  test           - Run tests"
	@echo "  test-coverage  - Run tests with coverage report"
	@echo "  lint           - Run linter (requires golangci-lint)"
	@echo "  tidy           - Tidy dependencies"
	@echo "  clean          - Clean build artifacts"
	@echo "  up / down      - Start / stop the Docker stack"
	@echo "  migrate-up     - Apply pending migrations"
	@echo "  migrate-down   - Roll back the last migration"
	@echo "  import-olist   - Import Olist CSVs into sfd (OLIST_DIR=../data/olist)"
	@echo "  seed-demo      - Create demo accounts seller_demo / buyer_demo"
	@echo "  swagger        - Generate swagger docs"
