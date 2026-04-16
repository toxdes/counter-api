.PHONY: build run test clean migrate-up migrate-down docs docs-generator version bump bump-minor bump-major help

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

bump:
	@echo "Bumping patch version..."
	@CURRENT=$$(cat version.txt) && \
	MAJOR=$$(echo $$CURRENT | cut -d. -f1) && \
	MINOR=$$(echo $$CURRENT | cut -d. -f2) && \
	PATCH=$$(echo $$CURRENT | cut -d. -f3) && \
	NEW="$${MAJOR}.$${MINOR}.$$((PATCH + 1))" && \
	echo $$NEW > version.txt && \
	echo "Version bumped: $$CURRENT → $$NEW"

bump-minor:
	@echo "Bumping minor version..."
	@CURRENT=$$(cat version.txt) && \
	MAJOR=$$(echo $$CURRENT | cut -d. -f1) && \
	MINOR=$$(echo $$CURRENT | cut -d. -f2) && \
	NEW="$${MAJOR}.$$((MINOR + 1)).0" && \
	echo $$NEW > version.txt && \
	echo "Version bumped: $$CURRENT → $$NEW"

bump-major:
	@echo "Bumping major version..."
	@CURRENT=$$(cat version.txt) && \
	MAJOR=$$(echo $$CURRENT | cut -d. -f1) && \
	NEW="$$(($$MAJOR + 1)).0.0" && \
	echo $$NEW > version.txt && \
	echo "Version bumped: $$CURRENT → $$NEW"

help:
	@echo "Available targets:"
	@echo "  build        - Build the application with version injection"
	@echo "  run          - Build and run the application"
	@echo "  test         - Run tests"
	@echo "  migrate-up   - Apply pending migrations"
	@echo "  migrate-down - Rollback last migration"
	@echo "  docs         - Generate HTML API documentation"
	@echo "  version      - Show application version"
	@echo "  bump         - Bump patch version (1.0.3 → 1.0.4)"
	@echo "  bump-minor   - Bump minor version (1.0.3 → 1.1.0)"
	@echo "  bump-major   - Bump major version (1.0.3 → 2.0.0)"
	@echo "  clean        - Clean build artifacts"
