# Makefile for warehouse_system

.PHONY: help run build test clean migrate-up migrate-down seed dev

help:
	@echo "Available targets:"
	@echo "  run          - Run the application"
	@echo "  dev          - Run with hot reload"
	@echo "  build        - Build production binary"
	@echo "  test         - Run tests"
	@echo "  test-cover   - Run tests with coverage"
	@echo "  migrate-up   - Run database migrations"
	@echo "  migrate-down - Rollback migrations"
	@echo "  seed         - Seed database"
	@echo "  clean        - Clean build artifacts"

run:
	@go run cmd/server/main.go

dev:
	@air || go run cmd/server/main.go

build:
	@mkdir -p bin
	@CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/warehouse_system cmd/server/main.go
	@echo "Built bin/warehouse_system"

test:
	@go test -v ./...

test-cover:
	@go test -v -coverprofile=coverage.out ./...
	@go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

migrate-up:
	@goose -dir internal/database/migrations postgres "$$(DATABASE_URL)" up

migrate-down:
	@goose -dir internal/database/migrations postgres "$$(DATABASE_URL)" down

seed:
	@go run cmd/seed/main.go

clean:
	@rm -rf bin/ dist/ coverage.out coverage.html
	@echo "Cleaned build artifacts"
