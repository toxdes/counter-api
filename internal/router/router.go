package router

import (
	"counter/internal/cache"
	"counter/internal/contract"
	"counter/internal/database"
	"counter/internal/handlers"
	"counter/internal/middleware"
	"counter/internal/service"
	"counter/internal/store"
	"net"
	"strconv"
	"strings"

	"github.com/qiangxue/fasthttp-routing"
	"github.com/valyala/fasthttp"
)

// Router holds the application router and dependencies.
type Router struct {
	fasthttp.RequestHandler
}

var routeSurface = []string{
	"GET /",
	"POST /tenants",
	"GET /tenants/<tenant_id>",
	"GET /tenants/<tenant_id>/counters",
	"POST /tenants/<tenant_id>/counters",
	"GET /tenants/<tenant_id>/counters/<counter_id>",
	"POST /tenants/<tenant_id>/counters/<counter_id>/inc",
	"POST /tenants/<tenant_id>/counters/<counter_id>/set",
	"POST /v2/tenants/<tenant_id>/counters/<counter_id>/inc",
	"POST /v2/tenants/<tenant_id>/counters/<counter_id>/set",
	"GET /v2/tenants/<tenant_id>/counters/<counter_id>/operations",
	"OPTIONS /*",
}

func sharedRouteSurface() []string {
	return append([]string(nil), routeSurface...)
}

// CORSMiddleware applies CORS headers to the request.
// Returns true if the request was fully handled by CORS (e.g., preflight OPTIONS), false if chain should continue.
func CORSMiddleware(c *routing.Context, corsConfig *middleware.CORSConfig) bool {
	handledByCORS := true
	corsHandler := middleware.CORS(corsConfig)(func(ctx *fasthttp.RequestCtx) {
		handledByCORS = false
	})
	corsHandler(c.RequestCtx)
	return handledByCORS
}

// LoggingMiddleware applies request logging.
func LoggingMiddleware(c *routing.Context, logger *middleware.Logger) {
	loggingHandler := middleware.Logging(nil)(func(ctx *fasthttp.RequestCtx) {})
	loggingHandler(c.RequestCtx)
}

// toHandler wraps fasthttp handlers for the routing library.
func toHandler(handler fasthttp.RequestHandler) routing.Handler {
	return func(c *routing.Context) error {
		// Don't execute the handler if a previous middleware already sent a response.
		if c.RequestCtx.Response.StatusCode() != fasthttp.StatusOK && c.RequestCtx.Response.StatusCode() != fasthttp.StatusNotFound {
			return nil
		}

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

// NewRouter creates a router with direct PostgreSQL-backed handlers.
func NewRouter(db *database.DB, corsConfig *middleware.CORSConfig, rateLimiter *middleware.RateLimiter, apiKey string, logger *middleware.Logger, sentryConfig *middleware.SentryConfig) *Router {
	return newRouter(db, nil, corsConfig, rateLimiter, apiKey, logger, sentryConfig)
}

// NewCachedRouter creates a router that overrides cache-eligible read handlers
// while sharing all middleware and route registration with NewRouter.
func NewCachedRouter(db *database.DB, cachedCounter *cache.CachedCounter, corsConfig *middleware.CORSConfig, rateLimiter *middleware.RateLimiter, apiKey string, logger *middleware.Logger, sentryConfig *middleware.SentryConfig) *Router {
	return newRouter(db, cachedCounter, corsConfig, rateLimiter, apiKey, logger, sentryConfig)
}

func newRouter(db *database.DB, cachedCounter *cache.CachedCounter, corsConfig *middleware.CORSConfig, rateLimiter *middleware.RateLimiter, apiKey string, logger *middleware.Logger, sentryConfig *middleware.SentryConfig) *Router {
	r := routing.New()
	installMiddleware(r, corsConfig, rateLimiter, logger, sentryConfig)

	tenantService := service.NewTenantService(store.NewTenantStore(db))
	counterStore := store.NewCounterStore(db)
	counterService := service.NewCounterService(counterStore)
	historyService := service.NewOperationHistoryService(counterStore)
	registerRoutes(r, apiKey, tenantService, counterService, historyService, cachedCounter)

	return &Router{RequestHandler: r.HandleRequest}
}

func installMiddleware(r *routing.Router, corsConfig *middleware.CORSConfig, rateLimiter *middleware.RateLimiter, logger *middleware.Logger, sentryConfig *middleware.SentryConfig) {
	// Rate limiting runs first so rejected requests do not perform later work.
	r.Use(func(c *routing.Context) error {
		ip := getClientIP(c.RequestCtx)
		isGet := string(c.RequestCtx.Method()) == "GET"
		allowed, retryAfter := rateLimiter.AllowRequest(ip, isGet)

		maxReq := rateLimiter.GetMaxRequests()
		if isGet {
			maxReq = rateLimiter.GetMaxGetRequests()
		}
		c.RequestCtx.Response.Header.Set("X-RateLimit-Limit", strconv.Itoa(maxReq))

		if !allowed {
			c.RequestCtx.Response.Header.Set("Retry-After", strconv.Itoa(retryAfter))
			c.RequestCtx.Response.Header.SetContentType("application/json")
			c.RequestCtx.SetStatusCode(fasthttp.StatusTooManyRequests)
			c.RequestCtx.SetBodyString(`{"error":"RATE_LIMIT_EXCEEDED","message":"Too many requests. Please retry later."}`)
			return nil
		}
		return c.Next()
	})

	if sentryConfig != nil && sentryConfig.DSN != "" {
		r.Use(func(c *routing.Context) error {
			sentryHandler := middleware.NewSentryHandler(func(ctx *fasthttp.RequestCtx) {})
			sentryHandler(c.RequestCtx)
			return c.Next()
		})
	}

	r.Use(func(c *routing.Context) error {
		if CORSMiddleware(c, corsConfig) {
			return nil
		}
		return c.Next()
	})
	r.Use(func(c *routing.Context) error {
		LoggingMiddleware(c, logger)
		return c.Next()
	})
}

func registerRoutes(r *routing.Router, apiKey string, tenantService service.TenantService, counterService service.CounterService, historyService service.OperationHistoryService, cachedCounter *cache.CachedCounter) {
	r.Get("/", toHandler(handlers.DocsHandler))

	r.Post("/tenants", middleware.APIKeyAuthRouting(apiKey)(toHandler(handlers.CreateTenantServiceHandler(tenantService))))
	r.Get("/tenants/<tenant_id>", toHandler(handlers.GetTenantServiceHandler(tenantService)))
	r.Get("/tenants/<tenant_id>/counters", middleware.APIKeyAuthRouting(apiKey)(toHandler(handlers.ListCountersServiceHandler(counterService))))
	r.Post("/tenants/<tenant_id>/counters", middleware.APIKeyAuthRouting(apiKey)(toHandler(handlers.CreateCounterServiceHandler(counterService))))

	getCounter := handlers.GetCounterServiceHandler(counterService)
	incrementCounter := handlers.IncrementCounterServiceHandler(counterService)
	setCounter := handlers.SetCounterServiceHandler(counterService)
	if cachedCounter != nil {
		getCounter = handlers.CachedGetCounterHandler(cachedCounter)
	}
	r.Get("/tenants/<tenant_id>/counters/<counter_id>", toHandler(getCounter))
	r.Post("/tenants/<tenant_id>/counters/<counter_id>/inc", toHandler(incrementCounter))
	r.Post("/tenants/<tenant_id>/counters/<counter_id>/set", middleware.APIKeyAuthRouting(apiKey)(toHandler(setCounter)))

	v2IncrementCounter := handlers.IncrementCounterServiceHandlerVersioned(counterService, contract.V2)
	v2SetCounter := handlers.SetCounterServiceHandlerVersioned(counterService, contract.V2)
	r.Post("/v2/tenants/<tenant_id>/counters/<counter_id>/inc", toHandler(v2IncrementCounter))
	r.Post("/v2/tenants/<tenant_id>/counters/<counter_id>/set", middleware.APIKeyAuthRouting(apiKey)(toHandler(v2SetCounter)))
	r.Get("/v2/tenants/<tenant_id>/counters/<counter_id>/operations", middleware.APIKeyAuthRouting(apiKey)(toHandler(handlers.OperationHistoryServiceHandler(historyService))))

	r.Options("/*", func(c *routing.Context) error {
		c.RequestCtx.SetStatusCode(fasthttp.StatusOK)
		return nil
	})
	r.NotFound(func(c *routing.Context) error {
		c.RequestCtx.SetStatusCode(fasthttp.StatusNotFound)
		c.RequestCtx.Response.Header.SetContentType("application/json")
		c.RequestCtx.SetBody([]byte(`{"error":"NOT_FOUND","message":"Endpoint not found"}`))
		return nil
	})
}

// ServeHTTP implements the fasthttp.RequestHandler interface.
func (r *Router) ServeHTTP(ctx *fasthttp.RequestCtx) {
	r.RequestHandler(ctx)
}

// getClientIP extracts the client IP from the request securely.
func getClientIP(ctx *fasthttp.RequestCtx) string {
	remoteIP := ctx.RemoteIP()
	if isTrustedProxy(remoteIP) {
		if ip := ctx.Request.Header.Peek("X-Real-IP"); len(ip) > 0 {
			if parsedIP := net.ParseIP(string(ip)); parsedIP != nil {
				return parsedIP.String()
			}
		}
		if ip := ctx.Request.Header.Peek("X-Forwarded-For"); len(ip) > 0 {
			ips := strings.Split(string(ip), ",")
			if len(ips) > 0 {
				if parsedIP := net.ParseIP(strings.TrimSpace(ips[0])); parsedIP != nil {
					return parsedIP.String()
				}
			}
		}
	}
	return remoteIP.String()
}

func isTrustedProxy(ip net.IP) bool {
	return ip != nil && ip.IsLoopback()
}
