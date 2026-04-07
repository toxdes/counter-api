.PHONY: build run test clean migrate-up migrate-down help

build:
	go build -o counter .

run: build
	./counter

test:
	go test -v -race ./...

migrate-up:
	@echo "Running migrations..."
	@for file in migrations/*.up.sql; do \
		echo "Running $$file..."; \
		psql -h localhost -U postgres -d counter_api -f "$$file"; \
	done

migrate-down:
	@echo "Rolling back migrations..."
	@for file in migrations/*.down.sql; do \
		echo "Running $$file..."; \
		psql -h localhost -U postgres -d counter_api -f "$$file"; \
	done

clean:
	rm -f counter
	rm -f *.log

help:
	@echo "Available targets:"
	@echo "  build        - Build the application"
	@echo "  run          - Build and run the application"
	@echo "  test         - Run tests"
	@echo "  migrate-up   - Apply pending migrations"
	@echo "  migrate-down - Rollback last migration"
	@echo "  clean        - Clean build artifacts"
