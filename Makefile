.PHONY: dev build run clean generate test install-tools migrate migrate-down migrate-create migrate-status

# Load .env file if it exists
-include .env
export

# Database connection string (override with environment variable or .env)
DATABASE_URL ?= postgres://postgres:postgres@localhost:5432/stashforme?sslmode=disable
MIGRATIONS_DIR = internal/database/migrations

# Install development tools
install-tools:
	go install github.com/a-h/templ/cmd/templ@latest
	go install github.com/air-verse/air@latest
	go install github.com/pressly/goose/v3/cmd/goose@latest

# Generate templ files
generate:
	templ generate

# Build the application
build: generate
	go build -o bin/server ./cmd/server

# Run the application
run: build
	./bin/server

# Development mode with hot reload
dev:
	air

# Run tests
test:
	go test ./...

# Clean build artifacts
clean:
	rm -rf bin tmp
	find . -name "*_templ.go" -delete

# Tidy dependencies
tidy:
	go mod tidy

# Run migrations
migrate:
	goose -dir $(MIGRATIONS_DIR) postgres "$(DATABASE_URL)" up

# Rollback last migration
migrate-down:
	goose -dir $(MIGRATIONS_DIR) postgres "$(DATABASE_URL)" down

# Create a new migration (usage: make migrate-create NAME=create_users)
migrate-create:
	goose -dir $(MIGRATIONS_DIR) create $(NAME) sql

# Show migration status
migrate-status:
	goose -dir $(MIGRATIONS_DIR) postgres "$(DATABASE_URL)" status
