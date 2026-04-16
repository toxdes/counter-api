package middleware

import (
	"strings"

	"github.com/valyala/fasthttp"
)

// sentryContext holds extracted request context for Sentry
type sentryContext struct {
	TenantID   string
	CounterID  string
	ClientIP   string
	Method     string
	Path       string
	StatusCode int
}

// extractSentryContext extracts tenant_id and counter_id from the request path
func extractSentryContext(ctx *fasthttp.RequestCtx) (tenantID, counterID string) {
	path := string(ctx.Path())

	// Parse path format: /tenants/{tenant_id}/counters/{counter_id}
	// or: /tenants/{tenant_id}
	parts := strings.Split(strings.Trim(path, "/"), "/")

	if len(parts) >= 2 && parts[0] == "tenants" {
		tenantID = parts[1]
	}
	if len(parts) >= 4 && parts[2] == "counters" {
		counterID = parts[3]
	}

	return tenantID, counterID
}

// shouldCaptureError returns true if the status code should be captured as an error
func shouldCaptureError(statusCode int) bool {
	// Capture 5xx server errors
	if statusCode >= 500 && statusCode < 600 {
		return true
	}
	// Capture 429 rate limit errors
	if statusCode == 429 {
		return true
	}
	return false
}
