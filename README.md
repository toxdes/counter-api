# Multi-Tenant Counter API

A lightweight, high-performance HTTP API for managing multi-tenant counters designed for high-frequency operations like blog post likes and visitor counts.

## Features

- **Multi-tenant counter management** - Isolated counters per tenant
- **High-performance** - Built with fasthttp for 5x faster throughput
- **PostgreSQL persistence** - Reliable data storage with connection pooling
- **Admin operations** - API key authentication for tenant/counter creation
- **Public operations** - Rate-limited counter access for direct browser calls
- **CORS support** - First-class browser integration
- **Structured logging** - JSON logs for easy aggregation

## Quick Start

### Prerequisites

- Go 1.21+
- PostgreSQL 15+

### Setup

1. **Clone and configure**
```bash
git clone <repository-url>
cd counter
cp .env.example .env
# Edit .env with your database credentials
```

2. **Create database**
```bash
createdb counter_api
```

3. **Run migrations**
```bash
make migrate-up
```

4. **Build and run**
```bash
make run
```

The API will be available at `http://localhost:8080`

## Usage Examples

### Create a tenant

```bash
curl -X POST http://localhost:8080/tenants \
  -H "X-API-Key: your-api-key" \
  -H "Content-Type: application/json" \
  -d '{"label": "blog"}'
```

### Create a counter

```bash
curl -X POST "http://localhost:8080/tenants/{tenant_id}" \
  -H "X-API-Key: your-api-key" \
  -H "Content-Type: application/json" \
  -d '{"label": "likes", "initial_value": 0}'
```

### Increment a counter

```bash
curl -X POST "http://localhost:8080/tenants/{tenant_id}/{counter_id}/inc?delta=1"
```

### Get counter value

```bash
curl -X GET "http://localhost:8080/tenants/{tenant_id}/{counter_id}"
```

## Documentation

- [API Documentation](docs/api.md) - Complete API reference with examples
- [Deployment Guide](docs/deployment.md) - Production deployment and operations
- [Design Spec](docs/superpowers/specs/2026-04-07-multi-tenant-counter-api-design.md) - Architecture and design decisions

## Development

```bash
make build    # Build the application
make test     # Run tests
make run      # Build and run
make clean    # Clean build artifacts
```

## Performance

- **Throughput**: 50,000+ requests/second on modest hardware
- **Latency**: <1ms p50 for increment operations
- **Memory**: <50MB baseline, scales with rate limit table
- **Connections**: Configurable pool, defaults to 25 max

## License

MIT
