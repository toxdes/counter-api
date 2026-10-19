# Counter API

A multi-tenant counter service backed by PostgreSQL. All data APIs use V2 and
require an API key with tenant/action authorization. Increments and value
adjustments are durable, recorded in operation history, and require an
idempotency key. Creation and credential lifecycle APIs support optional keys.

## Start locally

1. Install Go and PostgreSQL.
2. Copy `.env.example` to `.env` and set `DATABASE_URL` and `API_KEY`.
3. Run `make build`, `./counter --db-migrate=up`, then `./counter`.

## References

- [V2 API contract](docs/api-v2.md) — endpoint reference and V1 migration map.
- [API overview](docs/api.md) — permissions and idempotency behavior.
- [HTML API reference](docs/api.html) — generated endpoint overview.
- Run the server and open `/tools/curl` for a browser-only curl command builder.
- [Environment variables](.env.example) — configuration names and defaults.
- [Deployment guide](docs/deployment.md) — migrations, systemd, health checks,
  optional Redis, and production operations.
- [Nginx / Cloudflare guide](NGINX_DEPLOYMENT.md) — trusted client-IP forwarding.

## Compatibility and cache

Unversioned V1 data routes have been removed and return `404`. Internal tools
must use `/v2` URLs; tenant reads require `tenant:read`, and increments require
`counter:increment` plus a UUID idempotency key. There are no public data APIs.

PostgreSQL remains authoritative. V2 counter reads use a bounded read-through
cache; Redis is optional, with a per-process in-memory fallback. Cache size and
TTL are controlled by `COUNTER_READ_CACHE_MAX_ENTRIES` and
`COUNTER_READ_CACHE_TTL_SECONDS` (see `.env.example`).

## Development

Run `make test` for tests and `make build` to build the binary and refresh the
embedded HTML API documentation.
