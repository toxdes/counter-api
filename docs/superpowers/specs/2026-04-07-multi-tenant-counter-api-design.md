# Multi-Tenant Counter API Design

**Date:** 2026-04-07
**Status:** Approved
**Approach:** Pure fasthttp + sqlx

## Overview

A lightweight, high-performance HTTP API for managing multi-tenant counters designed for high-frequency operations like blog post likes and visitor counts. Built with Go, fasthttp, PostgreSQL, and optimized for maximum performance with minimal overhead.

## Use Case

The API serves as a backend for web applications that need to track and display counter values, such as:
- Like counts on blog posts
- Visitor/view counters for webpages
- General-purpose increment counters

Frontend applications call the API directly via `fetch()` from browsers, requiring CORS support and performance-optimized endpoints.

## Architecture

### System Design

```
┌─────────────────────────────────────────┐
│         fasthttp Server Entry           │
├─────────────────────────────────────────┤
│         Middleware Layer                │
│  ┌──────────┐  ┌─────────────────┐     │
│  │  CORS    │  │  Rate Limiter   │     │
│  │          │  │  (token bucket) │     │
│  └──────────┘  └─────────────────┘     │
│         ┌─────────────────────┐        │
│         │   API Key Auth      │        │
│         └─────────────────────┘        │
├─────────────────────────────────────────┤
│         Router Layer                    │
│  ┌──────────┐  ┌─────────────────┐     │
│  │ Tenant   │  │    Counter      │     │
│  │ Handlers │  │    Handlers     │     │
│  └──────────┘  └─────────────────┘     │
├─────────────────────────────────────────┤
│         Data Layer                      │
│  ┌──────────┐  ┌─────────────────┐     │
│  │Postgres  │  │  Rate Limit     │     │
│  │  sqlx    │  │   (in-memory)   │     │
│  └──────────┘  └─────────────────┘     │
└─────────────────────────────────────────┘
```

### Key Components

- **Server**: Single fasthttp instance with configurable host/port
- **Middleware Pipeline**: CORS → Rate Limiting → API Key Authentication (admin endpoints only)
- **Router**: Path-based routing to tenant and counter handlers
- **Data Layer**: sqlx with connection pooling, in-memory rate limiter

## API Specification

### Admin Endpoints (API Key Required)

All admin endpoints require `X-API-Key` header with the secret key from environment.

#### Create Tenant

**POST /tenants**

```json
Request:
{
  "label": "blog"
}

Response (201):
{
  "tenant_id": "01912345-6789-7000-8000-000000000001",
  "label": "blog",
  "created_at": "2026-04-07T12:00:00Z",
  "updated_at": "2026-04-07T12:00:00Z"
}

Error (409):
{
  "errors": [
    {
      "code": "TENANT_LABEL_EXISTS",
      "message": "A tenant with this label already exists"
    }
  ]
}
```

#### Create Counter

**POST /tenants/{tenant_id}**

```json
Request:
{
  "label": "post_likes",
  "initial_value": 0
}

Response (201):
{
  "counter_id": "01912345-6789-7000-8000-000000000002",
  "tenant_id": "01912345-6789-7000-8000-000000000001",
  "label": "post_likes",
  "value": 0,
  "created_at": "2026-04-07T12:00:00Z",
  "updated_at": "2026-04-07T12:00:00Z"
}

Error (404):
{
  "errors": [
    {
      "code": "TENANT_NOT_FOUND",
      "message": "Tenant not found"
    }
  ]
}
```

### Public Endpoints (Rate Limited)

Public endpoints are protected by IP-based rate limiting but do not require authentication.

#### Get Tenant

**GET /tenants/{tenant_id}**

```json
Response (200):
{
  "tenant_id": "01912345-6789-7000-8000-000000000001",
  "label": "blog",
  "created_at": "2026-04-07T12:00:00Z",
  "updated_at": "2026-04-07T12:00:00Z"
}
```

#### Increment Counter

**POST /tenants/{tenant_id}/{counter_id}/inc?delta=5**

- Query param `delta` defaults to `1` if not provided
- Delta must be a positive integer

```json
Response (200):
{
  "counter_id": "01912345-6789-7000-8000-000000000002",
  "value": 42,
  "updated_at": "2026-04-07T12:01:00Z"
}
```

#### Set Counter Value

**POST /tenants/{tenant_id}/{counter_id}/set?val=100**

- Query param `val` is required and must be an integer

```json
Response (200):
{
  "counter_id": "01912345-6789-7000-8000-000000000002",
  "value": 100,
  "updated_at": "2026-04-07T12:02:00Z"
}
```

#### Get Counter

**GET /tenants/{tenant_id}/{counter_id}**

```json
Response (200):
{
  "counter_id": "01912345-6789-7000-8000-000000000002",
  "tenant_id": "01912345-6789-7000-8000-000000000001",
  "label": "post_likes",
  "value": 42,
  "created_at": "2026-04-07T12:00:00Z",
  "updated_at": "2026-04-07T12:01:00Z"
}
```

## Database Schema

### Tables

```sql
CREATE TABLE tenants (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    label TEXT NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE counters (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    label TEXT,
    value BIGINT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_counters_tenant_id ON counters(tenant_id);
```

### Schema Design Decisions

- **UUID v7**: Time-ordered UUIDs for optimal database indexing performance
- **ON DELETE CASCADE**: Deleting a tenant automatically removes all associated counters
- **BIGINT for values**: Supports very large counter values
- **Composite index**: `idx_counters_tenant_id` optimizes the most common query pattern
- **Optional counter label**: The `label` field is nullable for counters
- **Tenant label uniqueness**: Enforced via UNIQUE constraint

### Timestamps

All tables include:
- `created_at`: Set on record creation, never updated
- `updated_at`: Set on creation, updated on any modification
- All timestamps in UTC, returned as ISO 8601 format in API responses

## Security Model

### Authentication

**Admin Endpoints** (tenant/counter creation):
- Protected by hardcoded API key from environment variable
- Client must include `X-API-Key` header with matching value
- Returns `401 Unauthorized` if missing or invalid

**Public Endpoints** (counter operations):
- No authentication required
- Protected by IP-based rate limiting
- Suitable for direct browser calls from frontend

### Rate Limiting

**Algorithm**: Token Bucket (in-memory, single-node)

**Configuration**:
```bash
RATE_LIMIT_REQUESTS=10    # Max requests per window
RATE_LIMIT_WINDOW=60      # Window in seconds
RATE_LIMIT_CLEANUP=300    # Cleanup interval for stale IPs
```

**Behavior**:
- Applied globally to ALL POST endpoints (both admin and public)
- Key: Client IP from `X-Real-IP` or `X-Forwarded-For` header (falls back to remote address)
- Returns `429 Too Many Requests` with `Retry-After` header when exceeded
- Background goroutine cleans up stale entries every 5 minutes
- Future: Replaceable with Redis for distributed systems

**Response Headers** (all requests):
```
X-RateLimit-Limit: 10
X-RateLimit-Remaining: 7
X-RateLimit-Reset: 1712485200
```

### CORS Configuration

**Environment Variables**:
```bash
CORS_ALLOWED_ORIGINS=https://example.com,https://*.example.com
CORS_ALLOWED_METHODS=GET,POST,OPTIONS
CORS_ALLOWED_HEADERS=Content-Type,Authorization,X-Request-ID
CORS_ALLOW_CREDENTIALS=false
CORS_MAX_AGE=3600
```

**Origin Matching**:
- Exact match: `https://example.com` matches only that origin
- Wildcard subdomains: `https://*.example.com` matches any subdomain
- Wildcard all: `*` allows any origin
- Validates `Origin` header on each request
- Returns matched origin in `Access-Control-Allow-Origin` header

## Error Handling

### Error Response Format

```json
{
  "errors": [
    {
      "code": "RATE_LIMIT_EXCEEDED",
      "message": "Rate limit exceeded. Try again in 30 seconds."
    }
  ]
}
```

Multiple errors can be returned for validation failures on multiple fields.

### HTTP Status Codes

- `200 OK` - Successful GET/POST request
- `201 Created` - Resource created successfully
- `400 Bad Request` - Invalid JSON, missing required fields, invalid query parameters
- `401 Unauthorized` - Missing or invalid API key (admin endpoints only)
- `404 Not Found` - Tenant or counter not found
- `409 Conflict` - Duplicate tenant label
- `422 Unprocessable Entity` - Validation failure
- `429 Too Many Requests` - Rate limit exceeded
- `500 Internal Server Error` - Unexpected server error

### Error Codes

- `TENANT_NOT_FOUND` - Tenant does not exist
- `TENANT_LABEL_EXISTS` - Tenant label already taken
- `COUNTER_NOT_FOUND` - Counter does not exist
- `RATE_LIMIT_EXCEEDED` - Rate limit exceeded
- `INVALID_API_KEY` - Invalid or missing API key
- `INVALID_JSON` - Malformed JSON in request body
- `INVALID_PARAMETER` - Invalid query or path parameter
- `INVALID_DELTA` - Delta must be a positive integer
- `INVALID_VALUE` - Value must be an integer

### Logging

**Structured Logging** (JSON format):
- Log levels: ERROR, WARN, INFO, DEBUG (configurable via `LOG_LEVEL`)
- All errors logged with: request ID, timestamp, endpoint, error details
- Request ID generated for each request, returned in `X-Request-ID` header

**Log Entry Example**:
```json
{
  "level": "ERROR",
  "time": "2026-04-07T12:00:00Z",
  "request_id": "req_123abc",
  "method": "POST",
  "path": "/tenants/abc/counters",
  "error": "database connection failed",
  "duration_ms": 5
}
```

## Environment Configuration

### Required Environment Variables

```bash
# Server Configuration
SERVER_HOST=0.0.0.0
SERVER_PORT=8080

# Database Configuration
DB_HOST=localhost
DB_PORT=5432
DB_USER=postgres
DB_PASSWORD=postgres
DB_NAME=counter_api
DB_SSL_MODE=disable
DB_MAX_OPEN_CONNS=25
DB_MAX_IDLE_CONNS=5

# Security
API_KEY=your-secret-api-key-here

# Rate Limiting
RATE_LIMIT_REQUESTS=10
RATE_LIMIT_WINDOW=60
RATE_LIMIT_CLEANUP=300

# CORS Configuration
CORS_ALLOWED_ORIGINS=https://example.com,https://*.example.com
CORS_ALLOWED_METHODS=GET,POST,OPTIONS
CORS_ALLOWED_HEADERS=Content-Type,Authorization,X-Request-ID
CORS_ALLOW_CREDENTIALS=false
CORS_MAX_AGE=3600

# Logging
LOG_LEVEL=info
```

### Environment Loading

- Uses `godotenv` library for .env file loading in development
- All environment variables have sensible defaults
- Required variables validated on startup (fails fast if missing)
- Production deployment can use real environment variables (no .env file needed)
- Example configuration provided in `.env.example`

## Project Structure

```
counter/
├── main.go                 # Application entry point
├── go.mod                  # Go module definition
├── go.sum                  # Dependency checksums
├── .env                    # Local environment (not in git)
├── .env.example            # Template for environment variables
├── .gitignore              # Git ignore rules
├── Makefile                # Common commands (build, run, test, migrate)
├── README.md               # Setup and usage documentation
├── migrations/
│   ├── 000001_init.up.sql        # Create tables
│   ├── 000001_init.down.sql      # Drop tables
│   └── schema_migrations.sql     # Migration tracking table
├── docs/
│   ├── api.md                     # API documentation (endpoints, examples)
│   └── deployment.md              # Deployment and production considerations
├── internal/
│   ├── config/
│   │   └── config.go             # Environment configuration loading
│   ├── database/
│   │   ├── db.go                 # Database connection & pooling
│   │   └── migrations.go         # Migration runner
│   ├── models/
│   │   ├── tenant.go             # Tenant data model
│   │   └── counter.go            # Counter data model
│   ├── handlers/
│   │   ├── tenant.go             # Tenant HTTP handlers
│   │   ├── counter.go            # Counter HTTP handlers
│   │   └── errors.go             # Error response helpers
│   ├── middleware/
│   │   ├── cors.go               # CORS middleware
│   │   ├── ratelimit.go          # Rate limiting middleware
│   │   ├── auth.go               # API key authentication
│   │   └── logging.go            # Request logging middleware
│   └── router/
│       └── router.go             # Route setup and handler registration
└── logs/                         # Log files (gitignored)
```

### Design Principles

- **Clear separation of concerns**: Config → Database → Handlers → Middleware → Router
- **Standard Go layout**: Uses `internal/` package for private code
- **Minimal dependencies**: Only essential packages for maximum performance
- **Fail fast**: Validation on startup, clear error messages
- **Testable**: Each component can be tested independently

## Technology Stack

### Core Technologies

- **Language**: Go 1.21+
- **HTTP Server**: fasthttp (high-performance HTTP server)
- **Database**: PostgreSQL 15+
- **Database Driver**: sqlx (extensions to database/sql)

### Key Dependencies

- `github.com/valyala/fasthttp` - Fast HTTP server
- `github.com/jmoiron/sqlx` - Database extensions
- `lib/pq` - PostgreSQL driver
- `github.com/joho/godotenv` - Environment variable loading
- `google.golang.org/uuid` - UUID v7 generation

### Migration System

Simple file-based migration system:
- Numbered SQL files in `migrations/` directory
- Up/down migrations for each version
- `schema_migrations` table tracks applied migrations
- Manual migration runner (no external migration tool dependency)

## Performance Considerations

### Database Optimization

- **UUID v7**: Time-ordered UUIDs for optimal index performance
- **Connection pooling**: Configurable pool size for PostgreSQL connections
- **Composite index**: `idx_counters_tenant_id` optimizes common queries
- **Simple queries**: Raw SQL via sqlx for maximum performance

### HTTP Server Optimization

- **fasthttp**: Zero-allocation HTTP server (5x faster than net/http)
- **Minimal middleware**: Only essential middleware in the pipeline
- **In-memory rate limiting**: No external service calls for rate checks
- **Connection reuse**: Persistent database connections

### Future Optimizations

- **Redis rate limiting**: For distributed deployments
- **Read replicas**: For high-read scenarios
- **Connection pooling**: Per-tenant connection pools if needed
- **Caching**: In-memory cache for frequently accessed counters

## Migration Strategy

### Initial Setup

1. Create `.env` file from `.env.example`
2. Configure database connection
3. Set API key for admin operations
4. Run migrations: `make migrate-up`
5. Start server: `make run`

### Schema Migrations

- Numbered migration files: `000001_init.up.sql`, `000002_add_indexes.up.sql`
- Up/down migrations for each change
- Migration status tracked in `schema_migrations` table
- Manual migration runner via `make migrate-up` and `make migrate-down`

## Development Workflow

### Makefile Commands

```makefile
make build        # Build the application
make run          # Build and run the application
make test         # Run tests
make migrate-up   # Apply pending migrations
make migrate-down # Rollback last migration
make clean        # Clean build artifacts
```

### Testing Strategy

- Unit tests for handlers, middleware, and business logic
- Integration tests with test database
- Load tests for rate limiting and concurrent operations
- Benchmark tests for performance validation

## Code Quality and Development Standards

### Go Best Practices

**Package Structure:**
- Follow standard Go project layout with `internal/` for private packages
- Keep packages focused and single-purpose
- Use clear, descriptive package names (e.g., `handlers`, `middleware`, `models`)
- Avoid circular dependencies

**Naming Conventions:**
- Use camelCase for exported functions and variables
- Use PascalCase for exported types, constants, and functions
- Use camelCase for unexported (private) declarations
- Use acronyms consistently (e.g., `HTTP` not `Http`, `ID` not `Id`)
- Interface names should end in `-er` when possible (e.g., `RateLimiter`, `Database`)

**Error Handling:**
- Always handle errors explicitly; never ignore them
- Wrap errors with context using `fmt.Errorf` or `errors.Wrap`
- Return errors from functions; don't panic in production code
- Use custom error types for expected error conditions
- Log errors at appropriate levels before returning them

**Concurrency:**
- Use goroutines sparingly and only when beneficial
- Always manage goroutine lifecycles properly
- Use channels or `sync.WaitGroup` for coordination
- Prefer channels over shared memory
- Use `sync.Mutex` or `sync.RWMutex` when shared state is necessary
- Be aware of goroutine leaks (always have exit conditions)

**Resource Management:**
- Always close resources (database connections, files, etc.)
- Use `defer` for cleanup operations
- Be mindful of connection pool limits
- Reuse objects where possible to reduce allocations

### Performance Optimization Principles

**Primary Focus:** Performance is the top priority, even when it requires additional complexity or development effort.

**Memory Allocation:**
- Minimize allocations in hot paths (request handlers, rate limiting)
- Use sync.Pool for object reuse (e.g., request buffers, response builders)
- Pre-allocate slices and maps with known capacity
- Avoid string concatenation in loops; use `strings.Builder`
- Use []byte instead of string for zero-copy operations where possible

**Database Operations:**
- Use prepared statements for repeated queries
- Batch operations when possible
- Select only needed columns; avoid `SELECT *`
- Use transactions for multi-step operations
- Consider read replicas for read-heavy workloads
- Optimize indexes based on query patterns

**HTTP Handling:**
- Reuse fasthttp request/response objects via pools
- Minimize middleware overhead
- Avoid unnecessary JSON encoding/decoding
- Use streaming for large responses
- Implement proper connection keep-alive

**Profiling and Optimization:**
- Use pprof for CPU and memory profiling
- Benchmark critical paths before and after changes
- Optimize based on actual profiling data, not assumptions
- Measure performance improvements with concrete metrics

### Code Comments and Documentation

**Comment Philosophy:** Short, concise comments that explain "why" not "what"

**When to Comment:**
- **DO** comment implicit assumptions and invariants
- **DO** comment performance-critical decisions
- **DO** comment non-obvious algorithms or workarounds
- **DO** comment external dependencies and their versions
- **DO** document exported functions, types, and constants
- **DON'T** repeat what the code already says
- **DON'T** comment obvious code
- **DON'T** use comments to explain bad code (refactor instead)

**Comment Style:**
- Use Go doc comments (`// FunctionName does...`) for exported identifiers
- Keep comments under 80 characters when possible
- Use present tense: "Returns" not "Returned"
- Use complete sentences for function documentation
- Include implicit assumptions in comments

**Example:**
```go
// RateLimiter implements token bucket rate limiting using in-memory storage.
// Assumes single-node deployment; for distributed systems, replace with Redis-backed implementation.
// Concurrent-safe for use across multiple goroutines.
type RateLimiter struct {
    // mu protects concurrent access to the store map
    mu sync.RWMutex
    // store maps client IP to token bucket; cleaned up every RATE_LIMIT_CLEANUP seconds
    store map[string]*tokenBucket
}
```

**Godoc Comments:**
- Every exported package should have a package comment
- Every exported function, type, and constant should have a doc comment
- Include examples in godoc for complex APIs
- Document all parameters and return values

**Implicit Assumptions to Document:**
- Database connection limits and pooling behavior
- Rate limit storage (in-memory vs distributed)
- Timestamp timezone assumptions (always UTC)
- ID generation strategies (UUID v7 for ordering)
- Thread-safety guarantees
- Performance characteristics of algorithms

## Documentation Structure

### API Documentation (docs/api.md)

The `docs/api.md` file serves as the primary API reference for consumers of this API. It includes:

**Contents:**
1. **Quick Start** - Basic setup and first request example
2. **Authentication** - How to use API keys for admin operations
3. **Endpoints** - Complete list of all endpoints with:
   - HTTP method and path
   - Authentication requirements
   - Request parameters (query, path, body)
   - Request/response examples
   - Error responses
   - Rate limiting information
4. **Error Codes** - Comprehensive list of error codes and meanings
5. **Rate Limiting** - How rate limiting works and header interpretation
6. **CORS** - CORS configuration and browser usage
7. **Examples** - Common use cases with code examples (curl, JavaScript, etc.)

**Format:**
- Markdown with clear sections
- Code examples in multiple languages
- Copy-paste ready curl commands
- JSON request/response examples
- Tables for quick reference

### Deployment Documentation (docs/deployment.md)

The `docs/deployment.md` file covers production deployment:

**Contents:**
1. **Requirements** - System requirements and dependencies
2. **Environment Setup** - Required environment variables and configuration
3. **Database Setup** - Database creation, migrations, and indexing
4. **Building** - How to build the binary for different platforms
5. **Running** - How to run the server (systemd, Docker, etc.)
6. **Production Checklist** - Security and performance settings
7. **Monitoring** - What to monitor and how
8. **Troubleshooting** - Common issues and solutions
9. **Scaling** - How to scale horizontally

**Format:**
- Step-by-step instructions
- Configuration examples
- Docker examples (if applicable)
- Performance tuning guidelines

### README.md

The project README includes:
- Project overview and purpose
- Quick start guide
- Features and capabilities
- Technology stack
- Basic usage examples
- Links to detailed documentation
- Development setup
- Contributing guidelines (if open source)

## Deployment Considerations

### Production Checklist

- Set strong API key in environment
- Configure CORS for production domains
- Enable database SSL (`DB_SSL_MODE=require`)
- Set appropriate rate limits
- Configure log level (WARN or ERROR for production)
- Set up log aggregation
- Configure database backups
- Monitor connection pool usage

### Scaling Considerations

- Single-node deployment with in-memory rate limiting
- For multi-instance deployments:
  - Replace in-memory rate limiter with Redis
  - Add load balancer in front of API instances
  - Use PostgreSQL read replicas for read-heavy workloads
- Consider CDN for API responses if global distribution needed

## Future Enhancements

### Planned Features

1. **Custom Rate Limiting Rules**: Per-tenant or per-counter rate limits
2. **Batch Operations**: Bulk counter increments/reads
3. **Counter Expiration**: TTL for temporary counters
4. **Webhook Support**: Notifications on counter thresholds
5. **Analytics Dashboard**: Real-time counter statistics
6. **Authentication Extensions**: JWT token support for multi-user scenarios

### Extension Points

The architecture supports easy addition of:
- New middleware components
- Additional counter operations (decrement, multiply)
- Custom rate limiting strategies
- Alternative authentication methods
- Event streaming for counter changes
