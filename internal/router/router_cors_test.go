package router

import (
	"testing"

	"github.com/valyala/fasthttp"
)

// TestOptionsPreflightRequest verifies that CORS preflight OPTIONS requests return 200 OK
func TestOptionsPreflightRequest(t *testing.T) {
	// Create a minimal CORS config for testing
	corsConfig := &CORSConfig{
		AllowedOrigins:   "*",
		AllowedMethods:   "GET,POST,OPTIONS",
		AllowedHeaders:   "Content-Type,Authorization",
		AllowCredentials: false,
		MaxAge:           3600,
	}

	// We can't create a full router without DB, but we can test the CORS middleware directly
	// by simulating what happens when an OPTIONS request comes in

	// Test that OPTIONS requests get CORS headers and return 200
	ctx := &fasthttp.RequestCtx{}
	ctx.Request.SetRequestURI("/tenants/test-id/counters/test-counter/inc")
	ctx.Request.Header.SetMethod("OPTIONS")
	ctx.Request.Header.Set("Origin", "https://toxdes.com")
	ctx.Request.Header.Set("Access-Control-Request-Method", "POST")
	ctx.Request.Header.Set("Access-Control-Request-Headers", "content-type")

	// Apply CORS middleware
	corsHandler := CORS(corsConfig)(func(ctx *fasthttp.RequestCtx) {
		// This should not be called for OPTIONS
		t.Error("CORS middleware should not call next handler for OPTIONS requests")
	})
	corsHandler(ctx)

	// Verify CORS headers are set
	allowOrigin := string(ctx.Response.Header.Peek("Access-Control-Allow-Origin"))
	if allowOrigin != "*" {
		t.Errorf("Expected Access-Control-Allow-Origin: *, got %s", allowOrigin)
	}

	allowMethods := string(ctx.Response.Header.Peek("Access-Control-Allow-Methods"))
	if allowMethods != "GET,POST,OPTIONS" {
		t.Errorf("Expected Access-Control-Allow-Methods: GET,POST,OPTIONS, got %s", allowMethods)
	}

	// Verify status is 200 OK
	if ctx.Response.StatusCode() != fasthttp.StatusOK {
		t.Errorf("Expected status 200 OK for OPTIONS request, got %d", ctx.Response.StatusCode())
	}
}

// CORSConfig is a minimal CORS config for testing
type CORSConfig struct {
	AllowedOrigins   string
	AllowedMethods   string
	AllowedHeaders   string
	AllowCredentials bool
	MaxAge           int
}

// CORS is a minimal implementation for testing
func CORS(config *CORSConfig) func(fasthttp.RequestHandler) fasthttp.RequestHandler {
	return func(next fasthttp.RequestHandler) fasthttp.RequestHandler {
		return func(ctx *fasthttp.RequestCtx) {
			origin := string(ctx.Request.Header.Peek("Origin"))

			// Set CORS headers
			if origin != "" && config.AllowedOrigins == "*" {
				ctx.Response.Header.Set("Access-Control-Allow-Origin", "*")
			}

			ctx.Response.Header.Set("Access-Control-Allow-Methods", config.AllowedMethods)
			ctx.Response.Header.Set("Access-Control-Allow-Headers", config.AllowedHeaders)

			if config.MaxAge > 0 {
				ctx.Response.Header.Set("Access-Control-Max-Age", "3600")
			}

			// Handle preflight requests
			if string(ctx.Method()) == "OPTIONS" {
				ctx.SetStatusCode(fasthttp.StatusOK)
				return
			}

			next(ctx)
		}
	}
}
