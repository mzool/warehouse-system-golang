# Makefile for tawseela

.PHONY: help run build test clean migrate migrate-status migrate-rollback migrate-dry-run migrate-force seed dev

help:
	@echo "Available targets:"
	@echo "  run             - Run the application"
	@echo "  dev             - Run with hot reload"
	@echo "  build           - Build production binary"
	@echo "  test            - Run tests"
	@echo "  test-cover      - Run tests with coverage"
	@echo "  migrate         - Apply pending database migrations"
	@echo "  migrate-status  - Show migration status"
	@echo "  migrate-rollback - Rollback last migration (use N=2 for multiple)"
	@echo "  migrate-dry-run - Preview migrations without applying"
	@echo "  migrate-force   - Force re-apply all migrations (dangerous!)"
	@echo "  seed            - Seed database"
	@echo "  clean           - Clean build artifacts"

run:
	@go run cmd/server/main.go

dev:
	@air || go run cmd/server/main.go

build:
	@mkdir -p bin
	@CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/tawseela cmd/server/main.go
	@echo "Built bin/tawseela"

test:
	@go test -v ./...

test-cover:
	@go test -v -coverprofile=coverage.out ./...
	@go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

migrate:
	@chmod +x scripts/migrate.sh
	@./scripts/migrate.sh

migrate-status:
	@chmod +x scripts/migrate.sh
	@./scripts/migrate.sh --status

migrate-rollback:
	@chmod +x scripts/migrate.sh
	@./scripts/migrate.sh --rollback $(or $(N),1)

migrate-dry-run:
	@chmod +x scripts/migrate.sh
	@./scripts/migrate.sh --dry-run

migrate-force:
	@chmod +x scripts/migrate.sh
	@echo "⚠️  WARNING: This will force re-apply all migrations!"
	@./scripts/migrate.sh --force

seed:
	@go run cmd/seed/main.go

clean:
	@rm -rf bin/ dist/ coverage.out coverage.html
	@echo "Cleaned build artifacts"

sqlc:
	@sqlc generate
	@echo "Generated SQL code with sqlc"