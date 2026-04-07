# Multi-Tenant Counter API

A lightweight, high-performance HTTP API for managing multi-tenant counters designed for high-frequency operations like blog post likes and visitor counts.

## Quick Start

### Prerequisites

- Go 1.21+
- PostgreSQL 15+

### Setup

1. Clone the repository
2. Copy environment template: `cp .env.example .env`
3. Configure database connection in `.env`
4. Create database: `createdb counter_api`
5. Run migrations: `make migrate-up`
6. Run server: `make run`

## Features

- Multi-tenant counter management
- High-performance fasthttp server
- PostgreSQL persistence with connection pooling
- API key authentication for admin operations
- IP-based rate limiting for public endpoints
- CORS support for browser clients
- Structured JSON logging

## Documentation

- [API Documentation](docs/api.md)
- [Deployment Guide](docs/deployment.md)

## Development

```bash
make build    # Build the application
make test     # Run tests
make run      # Build and run
```
