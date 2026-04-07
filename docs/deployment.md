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
SERVER_HOST=0.0.0.0
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

### 3. Run Migrations

```bash
make migrate-up
```

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

## Running

### Development

```bash
make run
```

### Production with systemd

Create `/etc/systemd/system/counter-api.service`:

```ini
[Unit]
Description=Counter API
After=network.target postgresql.service

[Service]
Type=simple
User=counterapi
WorkingDirectory=/opt/counter-api
ExecStart=/opt/counter-api/counter
Restart=always
RestartSec=5
EnvironmentFile=/opt/counter-api/.env

[Install]
WantedBy=multi-user.target
```

Enable and start:

```bash
sudo systemctl enable counter-api
sudo systemctl start counter-api
sudo systemctl status counter-api
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
COPY --from=builder /app/migrations ./migrations
COPY .env.example .env
EXPOSE 8080
CMD ["./counter"]
```

Build and run:

```bash
docker build -t counter-api .
docker run -d -p 8080:8080 --env-file .env counter-api
```

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

- **journald**: `./counter 2>&1 | systemd-cat -t counter-api`
- **file**: `./counter 2>&1 | tee -a /var/log/counter-api.log`
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
