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

API_KEY=your-random-secure-api-key-here

RATE_LIMIT_REQUESTS=10
RATE_LIMIT_WINDOW=60
RATE_LIMIT_CLEANUP=300

CORS_ALLOWED_ORIGINS=https://yourdomain.com
CORS_ALLOWED_METHODS=GET,POST,OPTIONS
CORS_ALLOWED_HEADERS=Content-Type,Authorization,X-Request-ID
CORS_ALLOW_CREDENTIALS=false
CORS_MAX_AGE=3600

LOG_LEVEL=warn
```

The API uses PostgreSQL as the sole authoritative counter path. The former
process-local cache and asynchronous write-behind queue are removed. Existing
`CACHE_*` environment variables are accepted but ignored with a deprecation
warning for one compatibility release; remove them from systemd, container,
and VPS configuration.

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

### Log Aggregation

Logs are output in JSON format. Send to:

- **journald**: `./counter 2>&1 | systemd-cat -t counter`
- **file**: `./counter 2>&1 | tee -a /var/log/counter.log`
- **cloudwatch**: Install AWS CloudWatch agent

### Health Checks

```bash
# Check if server is responding
curl -f http://localhost:8080/tenants/nonexistent || echo "Server down"

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

For multiple instances:

1. **Replace in-memory rate limiter with Redis**
   - Rate limit state must be shared
   - Use Redis INCR for atomic operations
   - TTL for automatic cleanup

2. **Add load balancer**
   - nginx, HAProxy, or cloud LB
   - Round-robin or least-connections
   - Health check endpoints

3. **Use PostgreSQL read replicas**
   - Direct read traffic to replicas
   - Write to primary only
   - Use connection pooling (PgBouncer)

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
