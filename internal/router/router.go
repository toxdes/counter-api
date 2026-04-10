package router

import (
	"counter/internal/cache"
	"counter/internal/database"
	"counter/internal/handlers"
	"counter/internal/middleware"
	"net"
	"strconv"
	"strings"

	"github.com/qiangxue/fasthttp-routing"
	"github.com/valyala/fasthttp"
)

// Router holds the application router and dependencies
type Router struct {
	fasthttp.RequestHandler
}

// CORSMiddleware applies CORS headers to the request
// Returns true if the request was fully handled by CORS (e.g., preflight OPTIONS), false if chain should continue
func CORSMiddleware(c *routing.Context, corsConfig *middleware.CORSConfig) bool {
	// Track whether the next handler is called
	// If called, CORS didn't handle the request (non-OPTIONS)
	// If not called, CORS handled it (preflight OPTIONS)
	handledByCORS := true

	corsHandler := middleware.CORS(corsConfig)(func(ctx *fasthttp.RequestCtx) {
		// This closure is only called for non-OPTIONS requests
		handledByCORS = false
	})
	corsHandler(c.RequestCtx)

	return handledByCORS
}

// LoggingMiddleware applies request logging
func LoggingMiddleware(c *routing.Context, logger *middleware.Logger) {
	// Apply Logging middleware
	loggingHandler := middleware.Logging(nil)(func(ctx *fasthttp.RequestCtx) {
		// Continue to next handler
	})
	loggingHandler(c.RequestCtx)
}

// toHandler wraps fasthttp handlers for routing library
func toHandler(handler fasthttp.RequestHandler) routing.Handler {
	return func(c *routing.Context) error {
		// Don't execute handler if response was already sent (e.g., rate limited)
		if c.RequestCtx.Response.StatusCode() != fasthttp.StatusOK && c.RequestCtx.Response.StatusCode() != fasthttp.StatusNotFound {
			return nil
		}

		// Copy routing parameters to request context for known parameters
		if tenantID := c.Param("tenant_id"); tenantID != "" {
			c.RequestCtx.SetUserValue("tenant_id", tenantID)
		}
		if counterID := c.Param("counter_id"); counterID != "" {
			c.RequestCtx.SetUserValue("counter_id", counterID)
		}
		handler(c.RequestCtx)
		return nil
	}
}

// NewRouter creates a new router with all routes and middleware
func NewRouter(db *database.DB, corsConfig *middleware.CORSConfig, rateLimiter *middleware.RateLimiter, apiKey string, logger *middleware.Logger) *Router {
	// Create router
	router := routing.New()

	// Apply global middleware as routing handlers
	// IMPORTANT: Order matters! Rate limiting should be FIRST to reject requests early
	router.Use(func(c *routing.Context) error {
		// Apply rate limiting directly - don't use wrapped middleware
		ip := getClientIP(c.RequestCtx)
		isGet := string(c.RequestCtx.Method()) == "GET"

		// Check if request should be allowed
		allowed, retryAfter := rateLimiter.AllowRequest(ip, isGet)

		// Set rate limit headers
		maxReq := rateLimiter.GetMaxRequests()
		if isGet {
			maxReq = rateLimiter.GetMaxGetRequests()
		}
		c.RequestCtx.Response.Header.Set("X-RateLimit-Limit", strconv.Itoa(maxReq))

		if !allowed {
			// Rate limit exceeded - reject immediately
			c.RequestCtx.Response.Header.Set("Retry-After", strconv.Itoa(retryAfter))
			c.RequestCtx.Response.Header.SetContentType("application/json")
			c.RequestCtx.SetStatusCode(fasthttp.StatusTooManyRequests)
			c.RequestCtx.SetBodyString(`{"error":"RATE_LIMIT_EXCEEDED","message":"Too many requests. Please retry later."}`)
			// Don't call c.Next() - stop the chain here
			return nil
		}

		// Request passed rate limit, continue to next middleware
		return c.Next()
	})

	router.Use(func(c *routing.Context) error {
		handled := CORSMiddleware(c, corsConfig)
		if handled {
			return nil // CORS handled this request (e.g., preflight OPTIONS)
		}
		return c.Next()
	})

	router.Use(func(c *routing.Context) error {
		LoggingMiddleware(c, logger)
		return c.Next()
	})

	// Admin endpoints (require API key)
	router.Post("/tenants", middleware.APIKeyAuthRouting(apiKey)(toHandler(handlers.CreateTenantHandler(db))))

	// Tenant endpoints
	router.Get("/tenants/<tenant_id>", toHandler(handlers.GetTenantHandler(db)))
	router.Post("/tenants/<tenant_id>/counters", middleware.APIKeyAuthRouting(apiKey)(toHandler(handlers.CreateCounterHandler(db))))

	// Counter endpoints
	router.Get("/tenants/<tenant_id>/counters/<counter_id>", toHandler(handlers.GetCounterHandler(db)))
	router.Post("/tenants/<tenant_id>/counters/<counter_id>/inc", toHandler(handlers.IncrementCounterHandler(db)))
	router.Post("/tenants/<tenant_id>/counters/<counter_id>/set", middleware.APIKeyAuthRouting(apiKey)(toHandler(handlers.SetCounterValueHandler(db))))

	// Custom 404 handler
	router.NotFound(func(c *routing.Context) error {
		c.RequestCtx.SetStatusCode(fasthttp.StatusNotFound)
		c.RequestCtx.Response.Header.SetContentType("application/json")
		c.RequestCtx.SetBody([]byte(`{"error":"NOT_FOUND","message":"Endpoint not found"}`))
		return nil
	})

	return &Router{RequestHandler: router.HandleRequest}
}

// ServeHTTP implements the fasthttp.RequestHandler interface
func (r *Router) ServeHTTP(ctx *fasthttp.RequestCtx) {
	r.RequestHandler(ctx)
}

// getClientIP extracts the client IP from the request securely
func getClientIP(ctx *fasthttp.RequestCtx) string {
	// IMPORTANT: Don't trust client-controlled headers for rate limiting
	remoteIP := ctx.RemoteIP()

	// Only trust headers from trusted proxies (localhost/private network)
	if isTrustedProxy(remoteIP) {
		// Try X-Real-IP first
		if ip := ctx.Request.Header.Peek("X-Real-IP"); len(ip) > 0 {
			parsedIP := net.ParseIP(string(ip))
			if parsedIP != nil {
				return parsedIP.String()
			}
		}

		// Try X-Forwarded-For (take first IP in chain)
		if ip := ctx.Request.Header.Peek("X-Forwarded-For"); len(ip) > 0 {
			ips := strings.Split(string(ip), ",")
			if len(ips) > 0 {
				parsedIP := net.ParseIP(strings.TrimSpace(ips[0]))
				if parsedIP != nil {
					return parsedIP.String()
				}
			}
		}
	}

	return remoteIP.String()
}

// isTrustedProxy checks if an IP is from a trusted proxy
func isTrustedProxy(ip net.IP) bool {
	if ip.IsLoopback() {
		return true
	}

	if ip.IsPrivate() {
		return true
	}

	// IPv4 private ranges
	if ip4 := ip.To4(); ip4 != nil {
		if ip4[0] == 10 {
			return true
		}
		if ip4[0] == 172 && ip4[1] >= 16 && ip4[1] <= 31 {
			return true
		}
		if ip4[0] == 192 && ip4[1] == 168 {
			return true
		}
	}

	return false
}

// NewCachedRouter creates a new router with caching enabled
func NewCachedRouter(db *database.DB, cachedCounter *cache.CachedCounter, corsConfig *middleware.CORSConfig, rateLimiter *middleware.RateLimiter, apiKey string, logger *middleware.Logger) *Router {
	// Create router
	router := routing.New()

	// Apply global middleware as routing handlers
	// IMPORTANT: Order matters! Rate limiting should be FIRST to reject requests early
	router.Use(func(c *routing.Context) error {
		// Apply rate limiting directly - don't use wrapped middleware
		ip := getClientIP(c.RequestCtx)
		isGet := string(c.RequestCtx.Method()) == "GET"

		// Check if request should be allowed
		allowed, retryAfter := rateLimiter.AllowRequest(ip, isGet)

		// Set rate limit headers
		maxReq := rateLimiter.GetMaxRequests()
		if isGet {
			maxReq = rateLimiter.GetMaxGetRequests()
		}
		c.RequestCtx.Response.Header.Set("X-RateLimit-Limit", strconv.Itoa(maxReq))

		if !allowed {
			// Rate limit exceeded - reject immediately
			c.RequestCtx.Response.Header.Set("Retry-After", strconv.Itoa(retryAfter))
			c.RequestCtx.Response.Header.SetContentType("application/json")
			c.RequestCtx.SetStatusCode(fasthttp.StatusTooManyRequests)
			c.RequestCtx.SetBodyString(`{"error":"RATE_LIMIT_EXCEEDED","message":"Too many requests. Please retry later."}`)
			// Don't call c.Next() - stop the chain here
			return nil
		}

		// Continue to next handler
		return c.Next()
	})

	// Apply CORS middleware
	router.Use(func(c *routing.Context) error {
		handled := CORSMiddleware(c, corsConfig)
		if handled {
			return nil // CORS handled this request (e.g., preflight OPTIONS)
		}
		return c.Next()
	})

	// Apply logging middleware
	router.Use(func(c *routing.Context) error {
		LoggingMiddleware(c, logger)
		return c.Next()
	})

	// Admin endpoints (require API key)
	router.Post("/tenants", middleware.APIKeyAuthRouting(apiKey)(toHandler(handlers.CreateTenantHandler(db))))

	// Tenant endpoints
	router.Get("/tenants/<tenant_id>", toHandler(handlers.GetTenantHandler(db)))
	router.Post("/tenants/<tenant_id>/counters", middleware.APIKeyAuthRouting(apiKey)(toHandler(handlers.CreateCounterHandler(db))))

	// Counter endpoints - use cached handlers
	router.Get("/tenants/<tenant_id>/counters/<counter_id>", toHandler(handlers.CachedGetCounterHandler(cachedCounter)))
	router.Post("/tenants/<tenant_id>/counters/<counter_id>/inc", toHandler(handlers.CachedIncrementCounterHandler(cachedCounter)))
	router.Post("/tenants/<tenant_id>/counters/<counter_id>/set", middleware.APIKeyAuthRouting(apiKey)(toHandler(handlers.CachedSetCounterValueHandler(cachedCounter))))

	// Custom 404 handler
	router.NotFound(func(c *routing.Context) error {
		c.RequestCtx.SetStatusCode(fasthttp.StatusNotFound)
		c.RequestCtx.Response.Header.SetContentType("application/json")
		c.RequestCtx.SetBody([]byte(`{"error":"NOT_FOUND","message":"Endpoint not found"}`))
		return nil
	})

	return &Router{RequestHandler: router.HandleRequest}
}
