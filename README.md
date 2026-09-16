# Multi-Tenant Counter API

A lightweight, high-performance HTTP API for managing multi-tenant counters designed for high-frequency operations like blog post likes and visitor counts.

## Features

- **Multi-tenant counter management** - Isolated counters per tenant
- **High-performance** - Built with fasthttp and PostgreSQL connection pooling
- **PostgreSQL persistence** - Reliable data storage with connection pooling
- **Admin operations** - API key authentication for tenant/counter creation
- **Public operations** - Rate-limited counter access for direct browser calls
- **CORS support** - First-class browser integration
- **Structured logging** - JSON logs for easy aggregation
- **Sentry integration** - Production-ready error tracking and monitoring
- **Graceful shutdown** - Stops serving cleanly on restart

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
- [Sentry Integration](docs/sentry.md) - Error tracking and monitoring setup
- [Design Spec](docs/superpowers/specs/2026-04-07-multi-tenant-counter-api-design.md) - Architecture and design decisions

## Development

```bash
make build    # Build the application
make test     # Run tests
make run      # Build and run
make clean    # Clean build artifacts
```

## Performance

The API reads and writes counters through PostgreSQL, which remains the sole
authoritative data path. Throughput and latency depend on the PostgreSQL
instance, connection-pool limits, request mix, and number of API replicas.
The default pool allows 25 open and 5 idle connections per process.

The former process-local cache and asynchronous write-behind queue have been
removed because they could acknowledge writes before persistence and diverge
between replicas. Existing `CACHE_*` environment variables are accepted but
ignored with a deprecation warning for one compatibility release; remove them
from deployment configuration.

## License

MIT
