.PHONY: build run test clean migrate-up migrate-down docs docs-generator help

build:
	go build -o counter .

run: build
	./counter

test:
	go test -v -race ./...

migrate-up:
	@echo "Running database migrations..."
	./counter --db-migrate=up

migrate-down:
	@echo "Rolling back database migrations..."
	./counter --db-migrate=down

clean:
	rm -f counter
	rm -f docs-generator
	rm -f *.log

docs: docs-generator
	@echo "Generating API documentation..."
	./docs-generator

docs-generator:
	@echo "Building docs generator..."
	go build -o docs-generator ./cmd/docs-generator

help:
	@echo "Available targets:"
	@echo "  build        - Build the application"
	@echo "  run          - Build and run the application"
	@echo "  test         - Run tests"
	@echo "  migrate-up   - Apply pending migrations"
	@echo "  migrate-down - Rollback last migration"
	@echo "  docs         - Generate HTML API documentation"
	@echo "  clean        - Clean build artifacts"
