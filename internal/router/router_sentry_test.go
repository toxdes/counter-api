package router

import (
	"counter/internal/middleware"
	"testing"

	"github.com/valyala/fasthttp"
)

func TestRouterWithSentryConfig(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	defer cleanupTestDB(t, db)

	corsConfig := &middleware.CORSConfig{
		AllowedOrigins:   "*",
		AllowedMethods:   "GET,POST,OPTIONS",
		AllowedHeaders:   "Content-Type,Authorization",
		AllowCredentials: false,
		MaxAge:           3600,
	}

	rateLimiter := middleware.NewRateLimiter(10, 1, 60)
	logger := middleware.NewDefaultLogger("info")

	tests := []struct {
		name         string
		sentryConfig *middleware.SentryConfig
		description  string
	}{
		{
			name:         "Router with nil Sentry config",
			sentryConfig: nil,
			description:  "Router should work without Sentry configuration",
		},
		{
			name: "Router with empty Sentry DSN",
			sentryConfig: &middleware.SentryConfig{
				DSN:         "",
				Environment: "test",
				Release:     "v1.0.0",
				SampleRate:  1.0,
			},
			description: "Router should work with empty Sentry DSN",
		},
		{
			name: "Router with valid Sentry config",
			sentryConfig: &middleware.SentryConfig{
				DSN:         "https://key@sentry.io/123",
				Environment: "test",
				Release:     "v1.0.0",
				SampleRate:  1.0,
			},
			description: "Router should work with valid Sentry configuration",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := NewRouter(db, corsConfig, rateLimiter, "test-key", logger, tt.sentryConfig)

			if router == nil {
				t.Error("Expected router to be created")
			}

			// Test that router can handle requests
			ctx := &fasthttp.RequestCtx{}
			ctx.Request.SetRequestURI("/tenants")
			ctx.Request.Header.SetMethod("GET")

			router.ServeHTTP(ctx)

			// Should get a response (either 200 or 404, but not crash)
			if ctx.Response.StatusCode() != fasthttp.StatusOK &&
			   ctx.Response.StatusCode() != fasthttp.StatusNotFound {
				t.Errorf("Expected status 200 or 404, got %d", ctx.Response.StatusCode())
			}
		})
	}
}

func TestCachedRouterWithSentryConfig(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	defer cleanupTestDB(t, db)

	corsConfig := &middleware.CORSConfig{
		AllowedOrigins:   "*",
		AllowedMethods:   "GET,POST,OPTIONS",
		AllowedHeaders:   "Content-Type,Authorization",
		AllowCredentials: false,
		MaxAge:           3600,
	}

	rateLimiter := middleware.NewRateLimiter(10, 1, 60)
	logger := middleware.NewDefaultLogger("info")

	tests := []struct {
		name         string
		sentryConfig *middleware.SentryConfig
		description  string
	}{
		{
			name:         "Cached router with nil Sentry config",
			sentryConfig: nil,
			description:  "Cached router should work without Sentry configuration",
		},
		{
			name: "Cached router with empty Sentry DSN",
			sentryConfig: &middleware.SentryConfig{
				DSN:         "",
				Environment: "test",
				Release:     "v1.0.0",
				SampleRate:  1.0,
			},
			description: "Cached router should work with empty Sentry DSN",
		},
		{
			name: "Cached router with valid Sentry config",
			sentryConfig: &middleware.SentryConfig{
				DSN:         "https://key@sentry.io/123",
				Environment: "test",
				Release:     "v1.0.0",
				SampleRate:  1.0,
			},
			description: "Cached router should work with valid Sentry configuration",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Note: We can't fully test cached router without a proper cache setup,
			// but we can test that it accepts the sentryConfig parameter
			router := NewCachedRouter(db, nil, corsConfig, rateLimiter, "test-key", logger, tt.sentryConfig)

			if router == nil {
				t.Error("Expected cached router to be created")
			}

			// Test that router can handle requests
			ctx := &fasthttp.RequestCtx{}
			ctx.Request.SetRequestURI("/tenants")
			ctx.Request.Header.SetMethod("GET")

			router.ServeHTTP(ctx)

			// Should get a response (either 200 or 404, but not crash)
			if ctx.Response.StatusCode() != fasthttp.StatusOK &&
			   ctx.Response.StatusCode() != fasthttp.StatusNotFound {
				t.Errorf("Expected status 200 or 404, got %d", ctx.Response.StatusCode())
			}
		})
	}
}

func TestRouterSentryMiddlewareChain(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	defer cleanupTestDB(t, db)

	corsConfig := &middleware.CORSConfig{
		AllowedOrigins:   "*",
		AllowedMethods:   "GET,POST,OPTIONS",
		AllowedHeaders:   "Content-Type,Authorization",
		AllowCredentials: false,
		MaxAge:           3600,
	}

	rateLimiter := middleware.NewRateLimiter(10, 1, 60)
	logger := middleware.NewDefaultLogger("info")

	sentryConfig := &middleware.SentryConfig{
		DSN:         "https://key@sentry.io/123",
		Environment: "test",
		Release:     "v1.0.0",
		SampleRate:  1.0,
	}

	router := NewRouter(db, corsConfig, rateLimiter, "test-key", logger, sentryConfig)

	// Test that all middleware are applied by checking rate limiting works
	// Create multiple requests to trigger rate limit
	for i := 0; i < 15; i++ {
		ctx := &fasthttp.RequestCtx{}
		ctx.Request.SetRequestURI("/tenants")
		ctx.Request.Header.SetMethod("GET")
		ctx.Request.Header.Set("X-Real-IP", "192.168.1.1")

		router.ServeHTTP(ctx)

		// After 10 requests, should get rate limited
		if i >= 10 {
			if ctx.Response.StatusCode() != fasthttp.StatusTooManyRequests {
				t.Errorf("Expected rate limit after 10 requests, got status %d on request %d",
					ctx.Response.StatusCode(), i+1)
			}
		}
	}
}
