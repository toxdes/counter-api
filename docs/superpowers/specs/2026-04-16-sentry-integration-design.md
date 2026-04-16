# Sentry Integration Design

**Date:** 2026-04-16
**Status:** Approved
**Author:** Claude

## Overview

Integrate Sentry error tracking and request logging into the fasthttp-based counter API. This integration will provide visibility into errors, panics, and request patterns without affecting application reliability.

## Requirements

1. **Request Logging**: Log each API request with HTTP method, path, status code, duration, and client IP
2. **Error Reporting**: Log 5xx server errors and 429 rate limit errors to Sentry
3. **Panic Recovery**: Capture panics with full stack traces
4. **Context Enrichment**: Add tenant_id and counter_id from request path for debugging
5. **Optional Configuration**: Sentry should be optional; fail gracefully if not configured
6. **Error Isolation**: Sentry failures must never affect application functionality

## Architecture

### Component Structure

```
internal/middleware/sentry.go  - Sentry middleware implementation
internal/config/config.go       - Configuration for Sentry
internal/router/router.go       - Middleware chain integration
main.go                         - Sentry initialization
```

### Middleware Chain Order

```
Rate Limiting → Sentry (panic recovery + transactions) → CORS → Logging → Handlers
```

**Rationale**: Placing Sentry after rate limiting ensures we don't generate noise from abusive clients, while still capturing all valid request traffic and errors.

### Request Lifecycle

1. **Request arrives** → Rate limiting check
2. **Passes rate limit** → Sentry middleware creates transaction
3. **Extract context** → tenant_id, counter_id, client IP from request
4. **Execute handler** → Wrapped with panic recovery
5. **Handler completes** → Finish transaction with status code
6. **Error check** → If status is 5xx or 429, capture error event
7. **Panic check** → If panic occurred, capture with stack trace and repanic

## Configuration

### Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `SENTRY_DSN` | No | (empty) | Sentry data source name |
| `SENTRY_ENVIRONMENT` | No | "development" | Environment name |
| `SENTRY_RELEASE` | No | (from build) | Release version |
| `SENTRY_SAMPLE_RATE` | No | 0.5 | Transaction sampling rate (0.0-1.0) |

### Configuration Structure

```go
type Config struct {
    // ... existing fields ...

    // Sentry
    SentryDSN         string
    SentryEnvironment string
    SentryRelease     string
    SentrySampleRate  float64
}
```

### Initialization Logic

- If `SENTRY_DSN` is empty: Skip Sentry initialization, log warning
- If `SENTRY_DSN` is provided: Initialize Sentry client with configuration
- All Sentry operations are wrapped with defensive error handling

## Sentry Middleware Design

### Key Types

```go
type SentryConfig struct {
    DSN         string
    Environment string
    Release     string
    SampleRate  float64
}

type sentryContext struct {
    TenantID   string
    CounterID  string
    ClientIP   string
    Method     string
    Path       string
    StatusCode int
}
```

### Main Functions

1. **`NewSentryHandler(config)`**: Creates the Sentry middleware handler
2. **`extractContext(ctx)`**: Extracts tenant_id, counter_id from request path
3. **`shouldCaptureError(statusCode)`**: Returns true for 5xx and 429
4. **`captureTransaction()`**: Creates and finishes Sentry transaction
5. **`recoverAndCapturePanic()`**: Recovers panics and captures to Sentry
6. **`safeCaptureError(hub, err)`**: Defensive error capture that never panics

### Middleware Flow

```go
func NewSentryHandler(config *SentryConfig) func(next fasthttp.RequestHandler) fasthttp.RequestHandler {
    return func(next fasthttp.RequestHandler) fasthttp.RequestHandler {
        return func(ctx *fasthttp.RequestCtx) {
            hub := GetHubFromContext(ctx)
            if hub == nil {
                next(ctx)
                return
            }

            // Extract context (tenant_id, counter_id, client_ip, etc.)
            sentryCtx := extractContext(ctx)

            // Create transaction
            transaction := startTransaction(hub, sentryCtx)
            defer finishTransaction(transaction, sentryCtx)

            // Set custom tags for context
            hub.Scope().SetTag("tenant_id", sentryCtx.TenantID)
            hub.Scope().SetTag("counter_id", sentryCtx.CounterID)
            hub.Scope().SetTag("client_ip", sentryCtx.ClientIP)

            defer func() {
                if r := recover(); r != nil {
                    // Capture panic with stack trace
                    safeCapturePanic(hub, r, sentryCtx)
                    panic(r) // Repanic for fasthttp to restart
                }
            }()

            next(ctx) // Execute handler chain

            // After request completes
            sentryCtx.StatusCode = ctx.Response.StatusCode()

            // Capture 5xx and 429 errors
            if shouldCaptureError(sentryCtx.StatusCode) {
                safeCaptureError(hub, sentryCtx)
            }
        }
    }
}
```

## Error Handling Strategy

### Error Categorization

| Error Type | Capture to Sentry? | Rationale |
|------------|-------------------|-----------|
| Panics | Yes (with stack trace) | Critical application errors |
| 5xx (500-599) | Yes | Server errors needing investigation |
| 429 Rate Limit | Yes | Abusive client behavior worth monitoring |
| 4xx (400-404) | No | Client errors, would create noise |
| 2xx/3xx | No | Successful requests |

### Panic Handling

- **Capture**: Use `sentry.Recover()` to capture stack trace
- **Context**: Include tenant_id, counter_id, client_ip, request path
- **Repanic**: Yes - fasthttp doesn't have built-in recovery, so repanic allows process restart
- **Wait for delivery**: Yes - ensures event sent before process dies

### Sentry SDK Error Handling

All Sentry operations are wrapped with defensive error handling:

```go
func safeCaptureError(hub *sentry.Hub, err error) {
    defer func() {
        if r := recover(); r != nil {
            log.Printf("Sentry capture panic: %v", r)
        }
    }()

    if hub != nil {
        hub.CaptureException(err)
    }
}
```

**Guarantees:**
- Sentry failures are logged locally but never propagated
- `nil` hub is handled gracefully (no-op)
- Panics in Sentry SDK are caught and logged

## Router Integration

### Integration Points

The Sentry middleware is integrated in both `NewRouter` and `NewCachedRouter`:

```go
router.Use(func(c *routing.Context) error {
    // 1. Rate Limiting (existing)
    // ...
    return c.Next()
})

router.Use(func(c *routing.Context) error {
    // 2. NEW: Sentry panic recovery + transactions
    sentryHandler := middleware.NewSentryHandler(sentryConfig)(func(ctx *fasthttp.RequestCtx) {
        // Continue to next middleware
    })
    sentryHandler(c.RequestCtx)
    return c.Next()
})

router.Use(func(c *routing.Context) error {
    // 3. CORS (existing)
    // ...
    return c.Next()
})

router.Use(func(c *routing.Context) error {
    // 4. Logging (existing)
    // ...
    return c.Next()
})
```

### Configuration Flow

```
main.go
  ↓ Load Config (includes Sentry fields)
  ↓ Create SentryConfig from Config
  ↓ Pass to router constructors
  ↓ Router creates Sentry middleware
```

## Testing Strategy

### Unit Tests (`internal/middleware/sentry_test.go`)

1. **`TestExtractContext`**
   - Valid path: `/tenants/abc123/counters/def456` → tenant_id=abc123, counter_id=def456
   - Partial path: `/tenants/abc123` → tenant_id=abc123, counter_id=""
   - No path params: `/health` → both empty

2. **`TestShouldCaptureError`**
   - 500 → true
   - 503 → true
   - 429 → true
   - 404 → false
   - 400 → false
   - 200 → false

3. **`TestPanicRecovery`**
   - Handler panics → panic captured, repanicked
   - Mock Sentry client to verify CaptureException called
   - Verify panic is rethrown

4. **`TestTransactionLifecycle`**
   - Request completes → transaction finished with correct status
   - Mock Sentry client to verify transaction created/finished

5. **`TestErrorIsolation`**
   - Sentry SDK failure → request still completes successfully
   - Hub is nil → no panic, graceful degradation

### Integration Tests

- Verify Sentry middleware is in the chain at the correct position
- Test that requests still succeed when Sentry is not configured (no DSN)
- Verify panic recovery works end-to-end
- Verify 5xx and 429 errors are captured

## Prerequisites

**IMPORTANT**: This implementation requires a prerequisite task to add version support:

1. Create `version.txt` containing `"1.0.0"`
2. Add `--version` flag to CLI
3. Build process: Inject version via ldflags at build time
4. Make version accessible at runtime for Sentry release tag

This must be completed before Sentry integration to provide accurate release information.

## Implementation Order

1. **Prerequisite**: Add version support (version.txt, build flags, --version flag)
2. **Configuration**: Add Sentry fields to Config struct
3. **Middleware**: Implement Sentry middleware with tests
4. **Router Integration**: Add Sentry middleware to router chain
5. **Initialization**: Initialize Sentry in main.go
6. **Testing**: Integration tests and manual verification

## Dependencies

Add to `go.mod`:
```go
require (
    github.com/getsentry/sentry-go v0.27.0
    github.com/getsentry/sentry-go/fasthttp v0.27.0
)
```

## Security Considerations

1. **PII in client IP**: Client IP is logged but only from trusted proxy headers (existing implementation)
2. **Tenant/Counter IDs**: These are API resource identifiers, not user data
3. **Request paths**: May expose API structure but not sensitive data
4. **DSN exposure**: DSN should be treated as a secret (environment variable)

## Performance Considerations

1. **Sampling**: Default 50% transaction sample rate reduces Sentry quota usage
2. **Async delivery**: Most operations are non-blocking
3. **Panic delivery**: Panics use WaitForDelivery to ensure capture before restart
4. **Error isolation**: Sentry failures don't block or slow down requests

## Success Criteria

- [ ] All panics are captured with stack traces
- [ ] All 5xx and 429 errors are captured to Sentry
- [ ] 4xx client errors (except 429) are not captured
- [ ] Transactions include method, path, status, duration, client IP
- [ ] Tenant ID and Counter ID are added as context when available
- [ ] Application continues working if Sentry is down or misconfigured
- [ ] Panics are repanicked to allow fasthttp restart
- [ ] Sentry is optional - works fine without DSN
- [ ] All unit tests pass
- [ ] Integration tests verify middleware chain order
