.PHONY: build run test clean migrate-up migrate-down docs docs-generator version help

VERSION ?= $(shell cat version.txt 2>/dev/null || echo "dev")
LDFLAGS = -X 'main.Version=$(VERSION)'

build:
	go build -ldflags="$(LDFLAGS)" -o counter-api .

run: build
	./counter-api

test:
	go test -v -race ./...

migrate-up:
	@echo "Running database migrations..."
	./counter-api --db-migrate=up

migrate-down:
	@echo "Rolling back database migrations..."
	./counter-api --db-migrate=down

clean:
	rm -f counter-api
	rm -f docs-generator
	rm -f *.log

docs: docs-generator
	@echo "Generating API documentation..."
	./docs-generator

docs-generator:
	@echo "Building docs generator..."
	go build -o docs-generator ./cmd/docs-generator

version:
	@echo "Counter API v$(VERSION)"

help:
	@echo "Available targets:"
	@echo "  build        - Build the application with version injection"
	@echo "  run          - Build and run the application"
	@echo "  test         - Run tests"
	@echo "  migrate-up   - Apply pending migrations"
	@echo "  migrate-down - Rollback last migration"
	@echo "  docs         - Generate HTML API documentation"
	@echo "  version      - Show application version"
	@echo "  clean        - Clean build artifacts"
