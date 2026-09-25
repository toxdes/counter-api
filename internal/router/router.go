package router

import (
	"counter/internal/contract"
	"counter/internal/database"
	"counter/internal/handlers"
	"counter/internal/middleware"
	"counter/internal/migrations"
	"counter/internal/observability"
	"counter/internal/service"
	"counter/internal/store"
	"net"

	"github.com/qiangxue/fasthttp-routing"
	"github.com/valyala/fasthttp"
)

// Router holds the application router and dependencies.
type Router struct {
	fasthttp.RequestHandler
}

var routeSurface = []string{
	"GET /",
	"GET /livez",
	"GET /readyz",
	"GET /metrics",
	"POST /tenants",
	"GET /tenants/<tenant_id>",
	"GET /tenants/<tenant_id>/counters",
	"POST /tenants/<tenant_id>/counters",
	"GET /tenants/<tenant_id>/counters/<counter_id>",
	"POST /tenants/<tenant_id>/counters/<counter_id>/inc",
	"POST /tenants/<tenant_id>/counters/<counter_id>/set",
	"POST /v2/tenants/<tenant_id>/counters/<counter_id>/inc",
	"POST /v2/tenants/<tenant_id>/counters/<counter_id>/set",
	"GET /v2/tenants/<tenant_id>/counters/<counter_id>",
	"GET /v2/tenants/<tenant_id>/counters/<counter_id>/operations",
	"POST /v2/tenants/<tenant_id>/credentials",
	"POST /v2/tenants/<tenant_id>/credentials/<credential_id>/rotate",
	"POST /v2/tenants/<tenant_id>/credentials/<credential_id>/revoke",
	"POST /v2/admin/credentials",
	"OPTIONS /*",
}

func sharedRouteSurface() []string {
	return append([]string(nil), routeSurface...)
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
		if credentialID := c.Param("credential_id"); credentialID != "" {
			c.RequestCtx.SetUserValue("credential_id", credentialID)
		}
		handler(c.RequestCtx)
		return nil
	}
}

// NewRouter creates a router with direct PostgreSQL-backed handlers.
func NewRouter(db *database.DB, corsConfig *middleware.CORSConfig, rateLimiter *middleware.RateLimiter, apiKey string, logger *middleware.Logger, sentryConfig *middleware.SentryConfig) *Router {
	return NewRouterWithOptions(db, corsConfig, rateLimiter, apiKey, true, logger, sentryConfig)
}

// NewRouterWithOptions allows deployments to disable the legacy global key
// after migrating protected clients to managed credentials.
func NewRouterWithOptions(db *database.DB, corsConfig *middleware.CORSConfig, rateLimiter *middleware.RateLimiter, apiKey string, legacyAPIKeyEnabled bool, logger *middleware.Logger, sentryConfig *middleware.SentryConfig) *Router {
	return NewRouterWithTimeouts(db, corsConfig, rateLimiter, apiKey, legacyAPIKeyEnabled, logger, sentryConfig, middleware.DefaultRouteTimeouts(), 128)
}

// NewRouterWithTimeouts adds explicit request and process concurrency budgets
// while preserving the older constructor for existing integrations/tests.
func NewRouterWithTimeouts(db *database.DB, corsConfig *middleware.CORSConfig, rateLimiter *middleware.RateLimiter, apiKey string, legacyAPIKeyEnabled bool, logger *middleware.Logger, sentryConfig *middleware.SentryConfig, timeouts middleware.RouteTimeouts, maxConcurrency int) *Router {
	state := observability.NewHealthState(migrations.LatestVersion)
	state.MarkStarted(migrations.LatestVersion)
	return NewRouterWithObservability(db, corsConfig, rateLimiter, apiKey, legacyAPIKeyEnabled, logger, sentryConfig, timeouts, maxConcurrency, OperationalOptions{
		Health:        state,
		Metrics:       observability.NewMetrics(),
		Version:       "dev",
		SchemaVersion: migrations.LatestVersion,
	})
}

type OperationalOptions struct {
	Health           *observability.HealthState
	Metrics          *observability.Metrics
	CounterReadCache service.CounterReadCache
	Version          string
	SchemaVersion    int64
}

func NewRouterWithObservability(db *database.DB, corsConfig *middleware.CORSConfig, rateLimiter *middleware.RateLimiter, apiKey string, legacyAPIKeyEnabled bool, logger *middleware.Logger, sentryConfig *middleware.SentryConfig, timeouts middleware.RouteTimeouts, maxConcurrency int, options OperationalOptions) *Router {
	if options.Health == nil {
		options.Health = observability.NewHealthState(migrations.LatestVersion)
	}
	if options.Metrics == nil {
		options.Metrics = observability.NewMetrics()
	}
	r := routing.New()
	tenantService := service.NewTenantService(store.NewTenantStore(db))
	counterStore := store.NewCounterStore(db)
	counterService := service.NewCounterServiceWithReadCache(counterStore, options.CounterReadCache)
	v1CounterReadService := service.NewCounterService(counterStore)
	historyService := service.NewOperationHistoryService(counterStore)
	credentialStore := store.NewCredentialStore(db)
	credentialService := service.NewCredentialService(credentialStore)
	registerRoutes(r, tenantService, counterService, v1CounterReadService, historyService, credentialService, options.Health, options.Metrics, db, options.Version, options.SchemaVersion)

	handler := fasthttp.RequestHandler(r.HandleRequest)
	handler = middleware.RateLimitWithMetrics(rateLimiter, options.Metrics)(handler)
	if !legacyAPIKeyEnabled {
		apiKey = ""
	}
	handler = middleware.AuthenticateRequest(middleware.NewAPIKeyAuthenticator(apiKey, credentialStore))(handler)
	handler = middleware.CORS(corsConfig)(handler)
	handler = middleware.ConcurrencyLimitWithMetrics(maxConcurrency, options.Metrics)(handler)
	handler = middleware.RequestContext(timeouts)(handler)
	handler = middleware.ClientIdentity(handler)
	handler = middleware.MetricsContext(options.Metrics)(handler)
	handler = middleware.LoggingWithLoggerAndMetrics(logger, options.Metrics)(handler)
	if sentryConfig != nil && sentryConfig.DSN != "" {
		handler = middleware.NewSentryHandler(handler)
	}
	handler = middleware.Recover(handler)

	return &Router{RequestHandler: handler}
}

func registerRoutes(r *routing.Router, tenantService service.TenantService, counterService, v1CounterReadService service.CounterService, historyService service.OperationHistoryService, credentialService service.CredentialService, health *observability.HealthState, metrics *observability.Metrics, db *database.DB, version string, schemaVersion int64) {
	r.Get("/", toHandler(handlers.DocsHandler))
	r.Get("/livez", toHandler(handlers.LivenessHandler(health)))
	r.Get("/readyz", toHandler(handlers.ReadinessHandler(health, db)))
	r.Get("/metrics", middleware.RequireAdminRouting(toHandler(handlers.MetricsHandler(metrics, db, version, schemaVersion))))

	r.Post("/tenants", middleware.RequireAdminRouting(toHandler(handlers.CreateTenantServiceHandler(tenantService))))
	r.Get("/tenants/<tenant_id>", toHandler(handlers.GetTenantServiceHandler(tenantService)))
	r.Get("/tenants/<tenant_id>/counters", middleware.RequireScopesRouting(middleware.ScopeCounterRead)(toHandler(handlers.ListCountersServiceHandler(counterService))))
	r.Post("/tenants/<tenant_id>/counters", middleware.RequireScopesRouting(middleware.ScopeCounterCreate)(toHandler(handlers.CreateCounterServiceHandler(counterService))))

	getCounter := handlers.GetCounterServiceHandler(v1CounterReadService)
	incrementCounter := handlers.IncrementCounterServiceHandler(counterService)
	setCounter := handlers.SetCounterServiceHandler(counterService)
	r.Get("/tenants/<tenant_id>/counters/<counter_id>", toHandler(getCounter))
	r.Post("/tenants/<tenant_id>/counters/<counter_id>/inc", toHandler(incrementCounter))
	r.Post("/tenants/<tenant_id>/counters/<counter_id>/set", middleware.RequireScopesRouting(middleware.ScopeCounterAdjust)(toHandler(setCounter)))

	v2IncrementCounter := handlers.IncrementCounterServiceHandlerVersioned(counterService, contract.V2)
	v2SetCounter := handlers.SetCounterServiceHandlerVersioned(counterService, contract.V2)
	r.Get("/v2/tenants/<tenant_id>/counters/<counter_id>", middleware.RequireScopesRouting(middleware.ScopeCounterRead)(toHandler(handlers.GetCounterServiceHandler(counterService))))
	r.Post("/v2/tenants/<tenant_id>/counters/<counter_id>/inc", middleware.RequireScopesRouting(middleware.ScopeCounterIncrement)(toHandler(v2IncrementCounter)))
	r.Post("/v2/tenants/<tenant_id>/counters/<counter_id>/set", middleware.RequireScopesRouting(middleware.ScopeCounterAdjust)(toHandler(v2SetCounter)))
	r.Get("/v2/tenants/<tenant_id>/counters/<counter_id>/operations", middleware.RequireScopesRouting(middleware.ScopeCounterHistory)(toHandler(handlers.OperationHistoryServiceHandler(historyService))))
	r.Post("/v2/tenants/<tenant_id>/credentials", middleware.RequireAdminRouting(toHandler(handlers.CreateCredentialServiceHandler(credentialService))))
	r.Post("/v2/tenants/<tenant_id>/credentials/<credential_id>/rotate", middleware.RequireAdminRouting(toHandler(handlers.RotateCredentialServiceHandler(credentialService))))
	r.Post("/v2/tenants/<tenant_id>/credentials/<credential_id>/revoke", middleware.RequireAdminRouting(toHandler(handlers.RevokeCredentialServiceHandler(credentialService))))
	r.Post("/v2/admin/credentials", middleware.RequireAdminRouting(toHandler(handlers.CreateAdminCredentialServiceHandler(credentialService))))

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

// getClientIP is retained for package-local compatibility with older tests.
func getClientIP(ctx *fasthttp.RequestCtx) string {
	return middleware.CanonicalClientIP(ctx)
}

func isTrustedProxy(ip net.IP) bool {
	return middleware.IsTrustedProxy(ip)
}
