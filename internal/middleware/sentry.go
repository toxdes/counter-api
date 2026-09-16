package middleware

import (
	"fmt"
	"strings"

	"github.com/getsentry/sentry-go"
	sentryfasthttp "github.com/getsentry/sentry-go/fasthttp"
	"github.com/valyala/fasthttp"
)

// SentryConfig holds configuration for Sentry middleware
type SentryConfig struct {
	DSN         string
	Environment string
	Release     string
	SampleRate  float64
}

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

		// Get the canonical visitor IP. The app receives requests from local
		// nginx, so RemoteIP alone would identify the proxy rather than the
		// visitor.
		clientIP := sentryClientIP(ctx)

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
				scope.SetTag("path", RouteTemplate(string(ctx.Path())))

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

// sentryClientIP returns the visitor identity established by the trusted
// loopback nginx proxy. Forwarding headers from any other peer are ignored.
func sentryClientIP(ctx *fasthttp.RequestCtx) string {
	return CanonicalClientIP(ctx)
}

// extractHeaders extracts HTTP headers from fasthttp context
func extractHeaders(ctx *fasthttp.RequestCtx) map[string]string {
	headers := make(map[string]string)
	allowed := map[string]string{
		"x-request-id": "X-Request-ID",
		"user-agent":   "User-Agent",
		"cf-ray":       "CF-Ray",
		"content-type": "Content-Type",
	}

	ctx.Request.Header.VisitAll(func(key, value []byte) {
		name, ok := allowed[strings.ToLower(string(key))]
		if !ok {
			return
		}

		// Keep attacker-controlled telemetry fields bounded.
		safeValue := string(value)
		if len(safeValue) > 256 {
			safeValue = safeValue[:256]
		}
		headers[name] = safeValue
	})
	return headers
}

// ScrubSentryEvent is a defense-in-depth scrubber for request data. The
// fasthttp Sentry integration attaches the full request before application
// middleware runs, so custom context filtering alone is insufficient.
func ScrubSentryEvent(event *sentry.Event, _ *sentry.EventHint) *sentry.Event {
	if event == nil {
		return nil
	}

	if request := event.Request; request != nil {
		request.Data = ""
		request.QueryString = ""
		request.Cookies = ""
		request.Env = nil
		request.Headers = scrubSentryHeaders(request.Headers)
		if queryStart := strings.IndexByte(request.URL, '?'); queryStart >= 0 {
			request.URL = request.URL[:queryStart]
		}
	}

	// The custom fasthttp context historically contained the full URL,
	// query string, and every request header. Retain only a safe method field;
	// route and identity are already represented by bounded tags.
	if event.Contexts != nil {
		if context, ok := event.Contexts["fasthttp"]; ok {
			safeContext := sentry.Context{}
			if method, ok := context["method"].(string); ok {
				safeContext["method"] = method
			}
			event.Contexts["fasthttp"] = safeContext
		}
	}

	return event
}

func scrubSentryHeaders(headers map[string]string) map[string]string {
	if len(headers) == 0 {
		return nil
	}

	allowed := map[string]string{
		"host":         "Host",
		"x-request-id": "X-Request-ID",
		"user-agent":   "User-Agent",
		"cf-ray":       "CF-Ray",
		"content-type": "Content-Type",
	}
	safeHeaders := make(map[string]string)
	for key, value := range headers {
		name, ok := allowed[strings.ToLower(key)]
		if !ok {
			continue
		}
		if len(value) > 256 {
			value = value[:256]
		}
		safeHeaders[name] = value
	}
	return safeHeaders
}
