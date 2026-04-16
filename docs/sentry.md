# Sentry Integration

This document describes the Sentry error tracking and performance monitoring integration for the Counter API.

## Overview

The Counter API integrates with Sentry for production-ready error tracking and performance monitoring. The integration is designed to be:

- **Optional**: Sentry is not required for the API to function
- **Non-blocking**: Sentry failures never impact API performance or availability
- **Defensive**: All Sentry operations are wrapped with error recovery
- **Context-aware**: Captures tenant and counter context for better debugging

## Features

- Automatic error capture for 5xx server errors
- Rate limit error tracking (429 status codes)
- Panic recovery and reporting
- Request context tracking (tenant_id, counter_id, client IP)
- Performance monitoring
- Configurable sampling rate to manage costs

## Configuration

Sentry integration is controlled via environment variables:

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `SENTRY_DSN` | No | empty | Your Sentry project DSN. If empty, Sentry is disabled |
| `SENTRY_ENVIRONMENT` | No | development | Environment name (e.g., production, staging) |
| `SENTRY_RELEASE` | No | empty | Release version for tracking deployments |
| `SENTRY_SAMPLE_RATE` | No | 0.5 | Fraction of transactions to sample (0.0 to 1.0) |

### Example Configuration

```bash
# Production Sentry setup
SENTRY_DSN=https://examplePublicKey@o0.ingest.sentry.io/0
SENTRY_ENVIRONMENT=production
SENTRY_RELEASE=1.0.0
SENTRY_SAMPLE_RATE=0.3
```

## What Gets Logged

### Always Logged

- **5xx Server Errors**: All internal server errors (500-599)
- **429 Rate Limit Errors**: Rate limit exceeded errors
- **Panics**: Unhandled panics in request handlers
- **Request Context**: Tenant ID, counter ID, client IP, method, path
- **Response Context**: Status codes for errors

### Never Logged

- **4xx Client Errors**: Bad requests (400-428, 430-499) are not captured
- **Successful Requests**: 2xx and 3xx status codes
- **Sensitive Data**: API keys, passwords, or sensitive request bodies
- **Request Bodies**: Payload content is not captured

## Error Isolation

Sentry integration is designed to be completely isolated from API operation:

### Initialization Failure

If Sentry fails to initialize (invalid DSN, network issues, etc.):
- ✅ API continues to function normally
- ✅ Errors are logged locally but not sent to Sentry
- ✅ No performance impact

### Runtime Failures

If Sentry operations fail during request handling:
- ✅ Request processing continues normally
- ✅ API responses are not affected
- ✅ Failures are silently ignored to prevent cascading issues

### Defensive Programming

All Sentry operations are wrapped with defensive error handling:
- Context extraction is panic-recovered
- Scope configuration is panic-recovered
- Error capture is panic-recovered
- Hub retrieval checks for nil before use

## Testing Sentry Locally

### 1. Set Up a Sentry Project

1. Create a free account at [sentry.io](https://sentry.io)
2. Create a new project selecting "Go" as the platform
3. Copy your DSN from the project settings

### 2. Configure Local Environment

Create a `.env.local` file in your project root:

```bash
# .env.local
SENTRY_DSN=https://yourPublicKey@o0.ingest.sentry.io/yourProjectID
SENTRY_ENVIRONMENT=development
SENTRY_RELEASE=dev
SENTRY_SAMPLE_RATE=1.0
```

### 3. Test Error Capture

Start the server and trigger errors:

```bash
# Build and run
make run

# Test version flag
./counter --version
# Output: Counter API v dev

# In another terminal, trigger a 500 error
curl -X POST http://localhost:8080/tenants/invalid-tenant-id/counters \
  -H "X-API-Key: your-api-key"

# Trigger rate limiting
for i in {1..20}; do
  curl http://localhost:8080/tenants/test
done
```

Check your Sentry dashboard for captured events.

### 4. Verify Configuration

```bash
# Check current version
./counter --version

# Verify Sentry is receiving events
# - Look for HTTP 500 errors in Sentry Issues
# - Check for rate limit (429) events
# - Verify tenant_id and counter_id tags appear
```

## Production Deployment

### 1. Set Production Environment Variables

```bash
SENTRY_DSN=https://yourPublicKey@o0.ingest.sentry.io/yourProjectID
SENTRY_ENVIRONMENT=production
SENTRY_RELEASE=1.0.0
SENTRY_SAMPLE_RATE=0.3
```

### 2. Configure Release Tracking

Build with version information:

```bash
# Build with version
go build -ldflags="-X main.Version=1.0.0" -o counter

# Set SENTRY_RELEASE to match
export SENTRY_RELEASE=1.0.0
```

### 3. Adjust Sample Rate

For high-traffic production environments:
- Start with `SENTRY_SAMPLE_RATE=0.1` (10% sampling)
- Increase to 0.3-0.5 for more detailed monitoring
- Set to 1.0 for full coverage (higher costs)

### 4. Monitor Sentry Health

- Check Sentry dashboard for error trends
- Set up alerts for spike in 5xx errors
- Monitor 429 rate limit events for capacity planning
- Track panic recovery events

## Troubleshooting

### Sentry Not Receiving Events

**Problem**: Events aren't appearing in Sentry dashboard

**Solutions**:
1. Verify `SENTRY_DSN` is set correctly
2. Check network connectivity to Sentry servers
3. Ensure `SENTRY_SAMPLE_RATE > 0`
4. Check application logs for initialization errors
5. Test with `SENTRY_SAMPLE_RATE=1.0` temporarily

### High Sentry Costs

**Problem**: Sentry usage is too expensive

**Solutions**:
1. Reduce `SENTRY_SAMPLE_RATE` (try 0.1 or 0.2)
2. Use environment-specific sampling (lower in production)
3. Set up Sentry quotas and alerts
4. Consider Sentry's on-premise option for large scale

### Performance Impact

**Problem**: API performance seems slower with Sentry

**Solutions**:
1. Verify `WaitForDelivery: false` is set (non-blocking)
2. Reduce `SENTRY_SAMPLE_RATE` to send fewer events
3. Check Sentry client isn't blocking on network operations
4. Monitor API latency metrics

### Context Not Appearing

**Problem**: tenant_id or counter_id tags are missing

**Solutions**:
1. Ensure request path follows `/tenants/{tenant_id}/counters/{counter_id}` format
2. Check that context extraction isn't panicking (check logs)
3. Verify hub is not nil during request handling
4. Test with manual curl requests to verify path format

### Panics Not Captured

**Problem**: Application panics aren't appearing in Sentry

**Solutions**:
1. Verify `Repanic: true` is set in Sentry handler
2. Check that fasthttp is using the Sentry handler
3. Ensure Sentry is initialized before server starts
4. Test with intentional panic to verify capture works

## Architecture

### Request Flow

```
Request → Sentry Handler → Extract Context → Configure Scope → Call Next Handler → Check Status → Capture Error → Response
```

### Error Capture Logic

```
Status Code >= 500 → Capture as server error
Status Code == 429 → Capture as rate limit error
Status Code < 500 → Don't capture (client errors)
Panic → Capture panic + set 500 status
```

### Context Extraction

```
Request Path → Parse tenant_id and counter_id → Set as tags → Add to user context → Include in event
```

## Best Practices

1. **Always set SENTRY_ENVIRONMENT**: Differentiate staging, production, dev
2. **Use release tracking**: Match SENTRY_RELEASE to your version
3. **Sample appropriately**: Start low (0.1), increase as needed
4. **Monitor 429 errors**: These indicate rate limit issues
5. **Set up alerts**: Get notified of error spikes
6. **Test locally**: Verify integration before deploying
7. **Keep DSN secure**: Use secrets management in production
8. **Review samples**: Periodically check sampled events for quality

## References

- [Sentry Go Documentation](https://docs.sentry.io/platforms/go/)
- [Sentry FastHTTP Integration](https://docs.sentry.io/platforms/go/guides/fasthttp/)
- [Sampling Documentation](https://docs.sentry.io/product/sentry-basics/concepts/)
- [Release Health](https://docs.sentry.io/product/releases/health/)
