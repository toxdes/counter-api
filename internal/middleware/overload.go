package middleware

import (
	"counter/internal/observability"
	"strconv"

	"github.com/valyala/fasthttp"
)

// ConcurrencyLimit is a bounded last-resort process-local safety valve. It
// rejects work before it can consume more memory or database connections.
// Shared/perimeter limits remain the authoritative control once replicas are
// deployed.
func ConcurrencyLimit(maxConcurrent int) func(fasthttp.RequestHandler) fasthttp.RequestHandler {
	return ConcurrencyLimitWithMetrics(maxConcurrent, nil)
}

func ConcurrencyLimitWithMetrics(maxConcurrent int, metrics *observability.Metrics) func(fasthttp.RequestHandler) fasthttp.RequestHandler {
	if maxConcurrent <= 0 {
		return func(next fasthttp.RequestHandler) fasthttp.RequestHandler { return next }
	}

	semaphore := make(chan struct{}, maxConcurrent)
	return func(next fasthttp.RequestHandler) fasthttp.RequestHandler {
		return func(ctx *fasthttp.RequestCtx) {
			select {
			case semaphore <- struct{}{}:
				defer func() { <-semaphore }()
				next(ctx)
			default:
				if metrics != nil {
					metrics.RecordEvent("overload_rejected")
				}
				ctx.Response.Header.Set("Retry-After", "1")
				ctx.Response.Header.Set("X-Overload-Limit", strconv.Itoa(maxConcurrent))
				ctx.Response.Header.SetContentType("application/json")
				ctx.SetStatusCode(fasthttp.StatusServiceUnavailable)
				ctx.SetBodyString(`{"errors":[{"code":"SERVICE_OVERLOADED","message":"Service is temporarily overloaded"}]}`)
			}
		}
	}
}
