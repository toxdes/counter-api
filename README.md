# Counter API

A multi-tenant counter service backed by PostgreSQL. Use V2 for new clients:
counter mutations are durable, recorded in operation history, and require an
idempotency key. Existing unversioned V1 routes remain available for
backward-compatible clients.

## Start locally

1. Install Go and PostgreSQL.
2. Copy `.env.example` to `.env` and set `DATABASE_URL` and `API_KEY`.
3. Run `make build`, `./counter --db-migrate=up`, then `./counter`.

## References

- [V2 API contract](docs/api-v2.md) — primary reference for new integrations.
- [V1 API reference](docs/api.md) — legacy, backward-compatible routes.
- [HTML API reference](docs/api.html) — generated endpoint overview.
- [Environment variables](.env.example) — configuration names and defaults.
- [Deployment guide](docs/deployment.md) — migrations, systemd, health checks,
  optional Redis, and production operations.
- [Nginx / Cloudflare guide](NGINX_DEPLOYMENT.md) — trusted client-IP forwarding.

## Compatibility and cache

V2 is additive; existing V1 consumers do not need to migrate immediately.
PostgreSQL remains authoritative. V2 counter reads use a bounded read-through
cache; Redis is optional, with a per-process in-memory fallback. Cache size and
TTL are controlled by `COUNTER_READ_CACHE_MAX_ENTRIES` and
`COUNTER_READ_CACHE_TTL_SECONDS` (see `.env.example`).

## Development

Run `make test` for tests and `make build` to build the binary and refresh the
embedded HTML API documentation.
