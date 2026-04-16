# Multi-Tenant Counter API

A lightweight, high-performance HTTP API for managing multi-tenant counters designed for high-frequency operations like blog post likes and visitor counts.

## Features

- **Multi-tenant counter management** - Isolated counters per tenant
- **High-performance caching** - In-memory LRU cache for ultra-fast reads and async writes
- **High-performance** - Built with fasthttp for 5x faster throughput
- **PostgreSQL persistence** - Reliable data storage with connection pooling
- **Admin operations** - API key authentication for tenant/counter creation
- **Public operations** - Rate-limited counter access for direct browser calls
- **CORS support** - First-class browser integration
- **Structured logging** - JSON logs for easy aggregation
- **Sentry integration** - Production-ready error tracking and monitoring
- **Graceful shutdown** - Ensures data integrity on restart

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

- **Throughput**: 100,000+ requests/second with caching enabled
- **Latency**: <1ms p50 for cached reads, ~5-10ms for database operations
- **Memory**: <50MB baseline + cache (configurable, ~1MB per 1000 cached counters)
- **Connections**: Configurable pool, defaults to 25 max

## Cache Configuration

The API includes an optional in-memory LRU cache for high-performance counter operations:

```bash
# Enable/disable cache (default: true)
CACHE_ENABLED=true

# Maximum number of counters to cache (default: 1000)
CACHE_SIZE=1000

# Cache entry TTL in seconds (default: 300)
CACHE_TTL_SECONDS=300

# Number of background workers for async writes (default: 2)
CACHE_WORKERS=2

# Write queue size (default: 10000)
CACHE_QUEUE_SIZE=10000

# Graceful shutdown wait time in seconds (default: 5)
CACHE_SHUTDOWN_WAIT=5
```

### Cache Behavior

- **GET requests** check cache first, falling back to database on miss
- **POST /inc** updates cache immediately and writes to database asynchronously
- **Cache eviction** uses LRU (Least Recently Used) algorithm
- **Graceful shutdown** drains pending writes before exiting

### Trade-offs

- ✅ **Performance**: 10-100x faster for cached operations
- ✅ **Database load**: Reduces database read operations significantly
- ⚠️ **Data loss**: Server crashes before async writes complete may lose recent increments
- ⚠️ **Memory usage**: Each cached counter consumes memory (~200 bytes)
- ⚠️ **Single instance**: Cache is per-instance, not distributed

## License

MIT
