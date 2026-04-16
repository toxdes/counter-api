package middleware

import (
	"fmt"
	"strings"

	"github.com/getsentry/sentry-go"
	sentryfasthttp "github.com/getsentry/sentry-go/fasthttp"
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

// NewSentryHandler creates a new Sentry middleware handler
// It wraps the provided handler with Sentry error tracking and performance monitoring
func NewSentryHandler(next fasthttp.RequestHandler) fasthttp.RequestHandler {
	// Create the Sentry FastHTTP handler with proper configuration
	sentryHandler := sentryfasthttp.New(sentryfasthttp.Options{
		Repanic:         true,  // Repanic after capturing to maintain fasthttp behavior
		WaitForDelivery: false, // Don't block on delivery for performance
	})

	// Return the wrapped handler
	return sentryHandler.Handle(func(ctx *fasthttp.RequestCtx) {
		// Get the Sentry hub from context (created by sentryfasthttp.Handle)
		hub := sentryfasthttp.GetHubFromContext(ctx)
		if hub == nil {
			// If no hub (Sentry not initialized), just call next handler
			next(ctx)
			return
		}

		// Extract context information with defensive error handling
		var tenantID, counterID string
		func() {
			defer func() {
				if r := recover(); r != nil {
					// If extraction panics, continue with empty values
					tenantID, counterID = "", ""
				}
			}()
			tenantID, counterID = extractSentryContext(ctx)
		}()

		// Get client IP
		clientIP := ctx.RemoteAddr().String()

		// Configure scope with request context (defensive)
		func() {
			defer func() {
				if r := recover(); r != nil {
					// If scope configuration panics, continue without it
				}
			}()

			hub.ConfigureScope(func(scope *sentry.Scope) {
				scope.SetTag("tenant_id", tenantID)
				scope.SetTag("counter_id", counterID)
				scope.SetTag("client_ip", clientIP)
				scope.SetTag("method", string(ctx.Method()))
				scope.SetTag("path", string(ctx.Path()))

				// Set user context if tenant_id is available
				if tenantID != "" {
					scope.SetUser(sentry.User{
						ID: tenantID,
					})
				}

				// Add custom context for fasthttp request
				scope.SetContext("fasthttp", map[string]interface{}{
					"url":         string(ctx.RequestURI()),
					"method":      string(ctx.Method()),
					"headers":     extractHeaders(ctx),
					"remote_addr": clientIP,
					"query":       string(ctx.QueryArgs().QueryString()),
				})
			})
		}()

		// Call the next handler with panic recovery
		func() {
			defer func() {
				if r := recover(); r != nil {
					// Panics are already captured by sentryfasthttp handler
					// Just set a 500 status if not already set
					if ctx.Response.StatusCode() == fasthttp.StatusOK {
						ctx.SetStatusCode(fasthttp.StatusInternalServerError)
					}
				}
			}()
			next(ctx)
		}()

		// After request completes, check if we should capture the error (defensive)
		func() {
			defer func() {
				if r := recover(); r != nil {
					// If error capture panics, just log and continue
				}
			}()

			if shouldCaptureError(ctx.Response.StatusCode()) {
				hub.WithScope(func(scope *sentry.Scope) {
					scope.SetLevel(sentry.LevelError)
					scope.SetContext("response", map[string]interface{}{
						"status_code": ctx.Response.StatusCode(),
					})

					// Capture HTTP error as event
					hub.CaptureMessage(fmt.Sprintf("HTTP %d: %s %s",
						ctx.Response.StatusCode(),
						ctx.Method(),
						ctx.Path()))
				})
			}
		}()
	})
}

// extractHeaders extracts HTTP headers from fasthttp context
func extractHeaders(ctx *fasthttp.RequestCtx) map[string]string {
	headers := make(map[string]string)
	ctx.Request.Header.VisitAll(func(key, value []byte) {
		headers[string(key)] = string(value)
	})
	return headers
}
