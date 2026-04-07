package router

import (
	"counter/internal/database"
	"counter/internal/handlers"
	"counter/internal/middleware"
	"os"

	"github.com/valyala/fasthttp"
)

// Router holds the application router and dependencies
type Router struct {
	fasthttp.RequestHandler
}

// Config holds router configuration
type Config struct {
	CORSConfig  *middleware.CORSConfig
	RateLimiter *middleware.RateLimiter
	APIKey      string
	Logger      *middleware.Logger
}

// NewRouter creates a new router with all routes and middleware
func NewRouter(db *database.DB, corsConfig *middleware.CORSConfig, rateLimiter *middleware.RateLimiter, apiKey string, logger *middleware.Logger) *Router {
	// Create router handler
	handler := fasthttp.CompressHandlerBrotliLevel(middleware.CORS(corsConfig)(
		middleware.RateLimit(rateLimiter)(
			middleware.Logging(os.Stdout)(func(ctx *fasthttp.RequestCtx) {
				switch string(ctx.Path()) {
				// Admin endpoints (require API key)
				case "/tenants":
					if string(ctx.Method()) == "POST" {
						middleware.APIKeyAuth(apiKey)(handlers.CreateTenantHandler(db))(ctx)
						return
					}
					ctx.SetStatusCode(fasthttp.StatusMethodNotAllowed)
					return

				// Tenant endpoints
				default:
					// Parse tenant ID from path
					path := string(ctx.Path())
					if len(path) > len("/tenants/") {
						tenantID := path[len("/tenants/"):]
						ctx.SetUserValue("tenant_id", tenantID)

						// Check if it's a counter path
						if idx := indexOf(path, "/counters/", len("/tenants/")); idx > 0 {
							// Extract counter ID
							counterIDStart := idx + len("/counters/")
							if counterIDEnd := indexOf(path, "/", counterIDStart); counterIDEnd > 0 {
								// Counter operation path
								counterID := path[counterIDStart:counterIDEnd]
								operation := path[counterIDEnd:]

								ctx.SetUserValue("counter_id", counterID)

								switch operation {
								case "/inc":
									if string(ctx.Method()) == "POST" {
										handlers.IncrementCounterHandler(db)(ctx)
										return
									}
								case "/set":
									if string(ctx.Method()) == "POST" {
										handlers.SetCounterValueHandler(db)(ctx)
										return
									}
								default:
									if string(ctx.Method()) == "GET" {
										handlers.GetCounterHandler(db)(ctx)
										return
									}
								}
							} else if string(ctx.Method()) == "GET" {
								// Get counter
								ctx.SetUserValue("counter_id", path[counterIDStart:])
								handlers.GetCounterHandler(db)(ctx)
								return
							}
						} else if string(ctx.Method()) == "POST" {
							// Create counter under tenant
							middleware.APIKeyAuth(apiKey)(handlers.CreateCounterHandler(db))(ctx)
							return
						} else if string(ctx.Method()) == "GET" {
							// Get tenant
							handlers.GetTenantHandler(db)(ctx)
							return
						}
					}

					ctx.SetStatusCode(fasthttp.StatusNotFound)
					return
				}
			}),
		),
	), 5, 5)

	return &Router{RequestHandler: handler}
}

// indexOf finds the index of a substring starting from a given position
func indexOf(s, substr string, start int) int {
	if start < 0 {
		start = 0
	}
	if start >= len(s) {
		return -1
	}
	for i := start; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

// ServeHTTP implements the fasthttp.RequestHandler interface
func (r *Router) ServeHTTP(ctx *fasthttp.RequestCtx) {
	r.RequestHandler(ctx)
}
