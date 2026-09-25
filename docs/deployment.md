# Deployment Guide

## Requirements

- Go 1.21+
- PostgreSQL 15+
- 512MB RAM minimum
- 1GB disk space

## Environment Setup

### 1. Create Database

```bash
sudo -u postgres psql
CREATE DATABASE counter_api;
CREATE USER counter_user WITH PASSWORD 'secure_password';
GRANT ALL PRIVILEGES ON DATABASE counter_api TO counter_user;
\q
```

### 2. Configure Environment

Create `.env` file:

```bash
# The API binds locally by default; expose it through nginx instead.
SERVER_HOST=127.0.0.1
SERVER_PORT=8080

DB_HOST=localhost
DB_PORT=5432
DB_USER=counter_user
DB_PASSWORD=secure_password
DB_NAME=counter_api
DB_SSL_MODE=disable
DB_MAX_OPEN_CONNS=25
DB_MAX_IDLE_CONNS=5
DB_MAX_IDLE_TIME_SECONDS=300
DB_TIMEOUT_MS=2000
DB_STATEMENT_TIMEOUT_MS=2000
DB_LOCK_TIMEOUT_MS=500
DB_IDLE_TRANSACTION_TIMEOUT_MS=10000

REQUEST_READ_TIMEOUT_SECONDS=5
REQUEST_MUTATION_TIMEOUT_SECONDS=10
SERVER_READ_TIMEOUT_SECONDS=10
SERVER_WRITE_TIMEOUT_SECONDS=10
SERVER_IDLE_TIMEOUT_SECONDS=30
SERVER_CONCURRENCY=128
SERVER_MAX_CONNS_PER_IP=100
MAX_REQUEST_BODY_BYTES=65536

API_KEY=your-random-secure-api-key-here
LEGACY_API_KEY_ENABLED=true

RATE_LIMIT_REQUESTS=10
RATE_LIMIT_WINDOW=60
RATE_LIMIT_CLEANUP=300

CORS_ALLOWED_ORIGINS=https://yourdomain.com
CORS_ALLOWED_METHODS=GET,POST,OPTIONS
CORS_ALLOWED_HEADERS=Content-Type,Authorization,X-Request-ID,X-API-Key,Idempotency-Key
CORS_ALLOW_CREDENTIALS=false
CORS_MAX_AGE=3600

LOG_LEVEL=warn
```

The API uses PostgreSQL as the sole authoritative counter path. The former
process-local cache and asynchronous write-behind queue are removed. Existing
`CACHE_*` environment variables are accepted but ignored with a deprecation
warning for one compatibility release; remove them from systemd, container,
and VPS configuration.

`API_KEY` is the legacy administrator/bootstrap credential. It is required for
tenant creation and credential lifecycle operations. New tenant credentials
are stored as one-way verifiers in PostgreSQL and should be created, rotated,
and revoked through the V2 credential endpoints. Set
`LEGACY_API_KEY_ENABLED=false` only after all protected clients have migrated
to managed credentials; keep it enabled during the compatibility period. To
disable it safely, first provision a managed administrator credential through
`POST /v2/admin/credentials`.

The request timeout is route-class based: reads default to five seconds and
mutations to ten seconds. Database operations receive a two-second budget,
while PostgreSQL also enforces statement, lock, and idle-transaction
timeouts. Keep `DB_MAX_OPEN_CONNS` within the database's global connection
budget when running more than one API replica; each replica's pool is additive.
The default 64 KiB body limit is intentionally sized for the JSON API rather
than file uploads.

Choose pool and concurrency values as a deployment profile, rather than
copying the defaults to every host:

| Profile | `DB_MAX_OPEN_CONNS` | `DB_MAX_IDLE_CONNS` | `SERVER_CONCURRENCY` | Intended use |
| --- | ---: | ---: | ---: | --- |
| Small VPS | 5 | 2 | 32 | 1 vCPU / 512 MiB host |
| Standard | 25 | 5 | 128 | Dedicated API instance |
| Replica set | Budget globally | Budget globally | 128+ | Multiple API instances sharing PostgreSQL |

For a replica set, divide the PostgreSQL connection budget across all API
instances and leave headroom for migrations, reconciliation, and operators.

The API rejects saturated in-process work with `503 SERVICE_OVERLOADED` and a
`Retry-After` header. Client quota exhaustion remains `429
RATE_LIMIT_EXCEEDED`. Nginx buffers request bodies and applies its own body,
connection, and upstream timeout limits before forwarding to Go.

### 3. Run Migrations

```bash
make migrate-up
```

To roll back one migration, run `make migrate-down`. Each invocation rolls
back only the highest applied migration; repeat it deliberately for additional
rollbacks.

To verify that materialized counter values match their completed operation
history, run `make reconcile`. The command scans counters in bounded batches,
reports mismatches and initial-value invariant violations, and exits non-zero
when inconsistencies are found. It does not modify application data.

### 4. Health and graceful deployment

The process verifies that the database is at the binary's supported migration
version before serving traffic. Configure the load balancer or nginx health
check against `/readyz`; use `/livez` only for process supervision. During a
shutdown the process marks itself unready first, stops accepting new listener
connections, drains bounded in-flight requests, and then closes PostgreSQL.

For a rolling deployment, remove the instance from `/readyz` traffic, send
SIGTERM, wait for the process to exit, and then replace it. Do not use sticky
sessions for correctness.

For a local multi-replica smoke topology, the repository includes three API
instances, PostgreSQL, and an HAProxy frontend with active `/readyz` checks:

```bash
docker compose -f deploy/scalability/compose.yaml up --build -d
docker compose -f deploy/scalability/compose.yaml ps
docker compose -f deploy/scalability/compose.yaml port load-balancer 8080
docker compose -f deploy/scalability/compose.yaml port api1 8080
docker compose -f deploy/scalability/compose.yaml port api2 8080
docker compose -f deploy/scalability/compose.yaml port api3 8080
```

Use the loopback address and assigned port from the last command to check
`/readyz`. The topology uses local-only credentials, binds HAProxy and each
API replica's diagnostic port to loopback, and gives each API instance five
PostgreSQL connections. Its named
database volume remains after `docker compose down`. Stop one API service to
observe health-check removal, then start it again:

```bash
docker compose -f deploy/scalability/compose.yaml stop api2
docker compose -f deploy/scalability/compose.yaml up -d api2
```

This is a development smoke setup, not a production deployment template.
`TestIdempotentIncrementCanRetryAcrossRouterReplicas` exercises a retry against
different router instances using the same PostgreSQL database.

### Repeatable load and soak runs

Run `scripts/scalability_load.py` from a separate machine or container against
the load-balancer URL. It creates uniquely named benchmark tenants/counters,
runs V2 increments with idempotency keys, and checks each counter's operation
history against its stored value. Benchmark records remain in the database, so
use a dedicated test database that can be reset between runs.
The runner rejects non-loopback API or metrics targets unless
`--allow-remote-target` is supplied explicitly.

```bash
API_KEY="$API_KEY" python3 scripts/scalability_load.py \
  --base-url http://127.0.0.1:<load-balancer-port> \
  --profile distributed \
  --tenants 10 --counters-per-tenant 10 \
  --requests 10000 --workers 32 --seed 1 \
  --metrics-url http://127.0.0.1:<api1-port> \
  --metrics-url http://127.0.0.1:<api2-port> \
  --metrics-url http://127.0.0.1:<api3-port> \
  --environment-json /path/to/run-environment.json
```

Profiles are `distributed`, `hot-tenant`, `hot-counter`, `read-heavy`, and
`retry-heavy`. Use `--duration-seconds 300` instead of `--requests` for a
five-minute saturation/soak run. `--seed` makes target selection and the
read/write mix repeatable. The report records the command, client platform,
configuration, HTTP status/error counts, throughput, latency percentiles,
private per-replica `/metrics` samples, and counter/history verification.
Pass each API replica's loopback base URL with a separate `--metrics-url`; the
load traffic itself should use the load balancer. `--environment-json` should
capture API and PostgreSQL replica counts and resource limits, PostgreSQL
version/settings, dataset notes, and storage type. For example:

```json
{
  "api_replicas": 3,
  "api_limits": {"cpu": "1 vCPU", "memory": "512 MiB"},
  "postgres": {
    "version": "17",
    "max_connections": 100,
    "settings": {"shared_buffers": "256MB"},
    "storage": "local SSD"
  },
  "dataset_notes": "fresh local test database"
}
```

Do not put credentials in this file. The report includes error examples; the
command line redacts an inline `--api-key`. Use the `by_kind` results to
separate read and increment latency in mixed runs. Each protected `/metrics`
sample also contains per-replica goroutine, Go heap, and cumulative GC data;
the runtime snapshot is collected when `/metrics` is scraped.

The script exits with status 2 if any counter's completed operation history
does not match the current value or the submitted successful operation IDs.
It reports HTTP failures separately from correctness, so a run can show
accepted throughput and errors while confirming the resulting state. For an
API process interruption, run a duration-based workload while killing and
restarting one local replica:

```bash
docker compose -f deploy/scalability/compose.yaml kill api2
docker compose -f deploy/scalability/compose.yaml up -d api2
```

Test PostgreSQL failover only in an HA test environment with the database
operator's failover procedure. Exercise row-lock contention on benchmark
counters, constrained connection pools, and network delay in an isolated test
environment, recording the injected condition and its start/end time in the
environment notes. The application reads counters from its configured writer;
record replication lag and avoid routing these validation reads to a stale
replica. Capture lock waits, WAL/checkpoint/I/O, replication lag, and host
CPU/memory from PostgreSQL and host monitoring alongside the JSON report.
Repeat the same command and compare tail latency and throughput only when
correctness passes.

Keep a capacity worksheet with each comparable report. Record average and
peak operations per second, the hottest counter's share of writes, p95/p99
latency targets and observed values, operation rows and WAL bytes per
increment, storage growth, database connection budget, replica count, and
monthly cost per million accepted increment operations. Use measured rates to
estimate daily history/WAL/storage growth and identify whether API CPU, writer
CPU/I/O, connection waits, or row-lock waits reaches its limit first.

Run the tool's unit tests with:

```bash
python3 -m unittest discover -s scripts -p 'test_scalability_load.py'
```

To include the optional shared rate-limit backend in the local topology, add
the Redis override:

```bash
docker compose -f deploy/scalability/compose.yaml \
  -f deploy/scalability/compose.redis.yaml up --build -d
docker compose -f deploy/scalability/compose.yaml \
  -f deploy/scalability/compose.redis.yaml port redis 6379
```

Use the assigned loopback port as `RATE_LIMIT_REDIS_TEST_URL` when running
`TestRedisSharedLimitAcrossIndependentClients`. Without Redis, each API replica
uses its local bounded limiter. With `RATE_LIMIT_REDIS_URL` configured, replicas
share rate-limit buckets through Redis. Redis is not used for counter state or
idempotency; if it cannot be reached, requests fall back to the local limiter
and the `rate_limit_backend_fallback` metric increases.

### 5. Backups and restore verification

Use managed PostgreSQL point-in-time recovery where available, with a recovery
point objective appropriate to the product. For self-managed PostgreSQL,
retain base backups and WAL archives with `pgBackRest` or an equivalent tool.
Take an encrypted custom-format backup for a portable smoke test:

```bash
pg_dump --format=custom --file=counter-api.dump "$DATABASE_URL"
```

Run the isolated restore check regularly from a backup host. It creates a
fresh database, restores the dump, runs reconciliation, and drops only that
fresh database:

```bash
RESTORE_TEST_DB=counter_api_restore_20260917 \
RESTORE_DATABASE_URL=postgres://counter_user:password@localhost/counter_api_restore_20260917 \
COUNTER_BIN=./counter \
./scripts/postgres_restore_smoke.py counter-api.dump
```

The restore is not considered successful unless reconciliation reports zero
mismatches and zero initial-value violations. Test both backup freshness and
restore completion alerts.

### PostgreSQL high availability and failover

The application supports both a single PostgreSQL instance and an HA
PostgreSQL deployment. Keep `DATABASE_URL` pointed at the logical writer
endpoint. For a managed service, this is the provider's writer endpoint; for a
self-managed deployment, it is the endpoint owned by the failover manager or
database proxy. Do not configure API replicas with a static hostname for an
individual primary.

An HA deployment should provide a primary and standby in separate failure
domains, automated promotion with fencing, and a tested writer endpoint. Use
synchronous or quorum replication when acknowledged writes must have an RPO
near zero; use asynchronous replication only when the documented replication
lag and possible loss window are acceptable. Replicas are not backups.

Keep counter mutations and read-after-write requests on the writer. Add read
replicas only for explicitly stale-tolerant reads, with separate routing and
monitoring. The API does not perform leader election or select database nodes.
Its `/readyz` check pings the configured writer and should remove an API
instance from load-balancer traffic when that writer is unavailable.

Use separate database roles for runtime traffic and migrations. The runtime
role should have only the required application DML permissions; the migration
job should use a separately managed credential with schema-change privileges.
Require TLS for database connections outside a trusted local network.

Failover validation must be exercised independently from backup restore:

1. Run retry-heavy increments against the API while failing the primary.
2. Confirm `/readyz` removes affected API paths and the writer endpoint is
   redirected to the promoted standby.
3. Retry failed mutations with the same V2 idempotency keys; do not blindly
   retry an unknown transaction outcome with a new key.
4. Measure time to recovery and actual data loss, then compare them with the
   declared RTO and RPO.
5. Run reconciliation after failover and after a restored-backup exercise.

Reset or recycle stale database connections as required by the managed service
or failover proxy. Keep the total `DB_MAX_OPEN_CONNS` budget across all API
replicas below the writer's safe capacity.

### 6. Release and supply-chain checks

Use a current patched Go toolchain for releases. The repository provides
optional checks for vulnerability scanning, secret scanning, SBOM generation,
and reproducible build flags:

```bash
make security
make sbom
make build-reproducible VERSION=1.0.0
```

The `security` target expects `govulncheck` and `gitleaks`; `sbom` expects
`syft`. Run them in CI and retain the SBOM and build metadata with the release
artifact. Do not commit generated SBOMs or secret-scan reports.

### 7. Initial SLOs

Use these as starting targets and revise them from production measurements:

| Signal | Initial target |
| --- | --- |
| Durable mutation availability | 99.9% monthly for committed V2 mutations |
| Mutation latency | p95 below 250 ms, p99 below 1 s |
| Reconciliation mismatch rate | 0 mismatches; alert on any mismatch |
| RPO | 5 minutes or better with WAL/PITR |
| RTO | 30 minutes or better for a regional database failure |

Track the targets separately from public V1 availability because V1 clients
may omit idempotency keys and cannot provide the same retry guarantees.

## Building

### Build for Linux

```bash
GOOS=linux GOARCH=amd64 go build -o counter .
```

### Build for macOS

```bash
GOOS=darwin GOARCH=amd64 go build -o counter .
```

### Build for Windows

```bash
GOOS=windows GOARCH=amd64 go build -o counter.exe .
```

## Testing

Run the normal unit and package tests with:

```bash
make test
```

PostgreSQL-backed integration tests use the real embedded migrations and an
isolated schema. Configure `TEST_DATABASE_URL` (or `DATABASE_URL`) and run:

```bash
make test-integration
```

The integration target fails when PostgreSQL is unavailable, making it suitable
for a CI gate. Local `go test` runs may skip those tests when no test database
is configured.

## Running

### Development

```bash
make run
```

### Production with systemd

Create `/etc/systemd/system/counter.service`:

```ini
[Unit]
Description=Counter API
After=network.target postgresql.service

[Service]
Type=simple
User=counterapi
WorkingDirectory=/opt/counter
ExecStart=/opt/counter/counter
Restart=always
RestartSec=5
EnvironmentFile=/opt/counter/.env

[Install]
WantedBy=multi-user.target
```

Enable and start:

```bash
sudo systemctl enable counter
sudo systemctl start counter
sudo systemctl status counter
```

### Production with Docker

Create `Dockerfile`:

```dockerfile
FROM golang:1.21-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -o counter .

FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /root/
COPY --from=builder /app/counter .
COPY .env.example .env
EXPOSE 8080
CMD ["./counter"]
```

The binary embeds the canonical migration history, so no migration directory
needs to be copied into the runtime image.

Build and run:

```bash
docker build -t counter .
docker run -d -p 8080:8080 --env-file .env counter
```

When the API is running inside a container, set `SERVER_HOST=0.0.0.0` in the
container environment so the published port or a sidecar nginx can reach it.
Keep the default `127.0.0.1` for a host-level deployment behind nginx.

## Production Checklist

- [ ] Set strong API key
- [ ] Configure CORS for production domains only
- [ ] Enable database SSL (`DB_SSL_MODE=require`)
- [ ] Set appropriate rate limits
- [ ] Configure log level to WARN or ERROR
- [ ] Set up log aggregation (e.g., journald, cloudwatch)
- [ ] Configure database backups
- [ ] If HA is required, configure managed PostgreSQL or an owned failover manager
- [ ] Declare and test PostgreSQL RPO/RTO
- [ ] Monitor replication lag, WAL retention, failover state, and writer connectivity
- [ ] Set up monitoring for connection pool usage
- [ ] Configure reverse proxy (nginx) for HTTPS
- [ ] Set up process monitoring (systemd, supervisord)

## Monitoring

### Key Metrics

- Request rate and response times
- Database connection pool usage
- Rate limit violations
- Error rates by endpoint
- Memory and CPU usage
- PostgreSQL replication lag, WAL retention, replay state, and failover events

### Log Aggregation

Logs are output in JSON format. Send to:

- **journald**: `./counter 2>&1 | systemd-cat -t counter`
- **file**: `./counter 2>&1 | tee -a /var/log/counter.log`
- **cloudwatch**: Install AWS CloudWatch agent

### Health Checks

```bash
# Process health; does not require PostgreSQL
curl -f http://localhost:8080/livez

# Traffic readiness; requires compatible schema and PostgreSQL
curl -f http://localhost:8080/readyz

# Administrator-protected Prometheus metrics
curl -H "X-API-Key: $API_KEY" http://localhost:8080/metrics

# Check database connectivity
psql -h localhost -U counter_user -d counter_api -c "SELECT 1"
```

## Scaling

### Vertical Scaling

Increase resources:
- More RAM for larger connection pools
- Faster CPU for higher request throughput
- SSD storage for faster database queries

### Horizontal Scaling

Run stateless API replicas behind an external load balancer and keep PostgreSQL
as the source of truth. The API does not implement load balancing. Do not use
sticky sessions; retries can reach any replica because mutation idempotency is
stored in PostgreSQL.

The default token bucket lives in each API process, so its effective allowance
is per replica and multiplies as replicas are added. Set `RATE_LIMIT_REDIS_URL`
to share tenant/key buckets across replicas; Redis is optional, and an outage
falls back to each process's local bounded limiter. Use edge controls for
coarse shared IP limits. Counter correctness and idempotency remain PostgreSQL
backed and do not depend on Redis. Budget all replica connection pools together
and keep read-after-write traffic on the PostgreSQL writer.

## Troubleshooting

### Database Connection Errors

```
failed to connect to database: connection refused
```

- Verify PostgreSQL is running: `sudo systemctl status postgresql`
- Check connection settings in `.env`
- Verify firewall allows port 5432
- Check `pg_hba.conf` for authentication settings

### High Memory Usage

- Reduce `DB_MAX_OPEN_CONNS`
- Reduce `RATE_LIMIT_REQUESTS` window
- Profile with `pprof`: `import _ "net/http/pprof"`

### Slow Response Times

- Check database query performance: `EXPLAIN ANALYZE`
- Add indexes on frequently queried columns
- Increase connection pool size
- Check for network latency

### Rate Limit Issues

```
429 Too Many Requests
```

- Increase `RATE_LIMIT_REQUESTS`
- Decrease `RATE_LIMIT_WINDOW`
- Check for malicious traffic patterns
- Consider IP whitelisting for known clients

## Security Considerations

### API Key Management

- Rotate API keys regularly
- Use strong, randomly generated keys
- Never commit keys to git
- Use different keys for dev/staging/prod

### Database Security

- Use strong passwords
- Enable SSL for database connections
- Restrict network access to localhost
- Regular security updates

### Network Security

- Use HTTPS in production (reverse proxy)
- Configure CORS restrictively
- Implement rate limiting
- Monitor for abuse patterns
- Use firewall to restrict access
