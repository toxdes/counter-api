package handlers

import (
	"context"
	"counter/internal/database"
	"counter/internal/middleware"
	"counter/internal/observability"
	"time"

	"github.com/valyala/fasthttp"
)

// DatabasePinger is the database capability required by readiness checks.
// Keeping this boundary small allows failover behavior to be tested without
// requiring a concrete database pool in the handler tests.
type DatabasePinger interface {
	PingContext(context.Context) error
}

func LivenessHandler(state *observability.HealthState) fasthttp.RequestHandler {
	return func(ctx *fasthttp.RequestCtx) {
		if state == nil || !state.Live() {
			respondWithError(ctx, fasthttp.StatusServiceUnavailable, "NOT_LIVE", "Service is not live")
			return
		}
		respondWithJSON(ctx, fasthttp.StatusOK, map[string]string{"status": "ok"})
	}
}

func ReadinessHandler(state *observability.HealthState, db DatabasePinger) fasthttp.RequestHandler {
	return func(ctx *fasthttp.RequestCtx) {
		requestContext, _ := middleware.RequestContextFromRequest(ctx)
		checkContext, cancel := context.WithTimeout(requestContext, time.Second)
		defer cancel()
		var ping func(context.Context) error
		if db != nil {
			ping = db.PingContext
		}
		if err := state.CheckReady(checkContext, ping); err != nil {
			respondWithError(ctx, fasthttp.StatusServiceUnavailable, "NOT_READY", "Service is not ready")
			return
		}
		respondWithJSON(ctx, fasthttp.StatusOK, map[string]string{"status": "ok"})
	}
}

func MetricsHandler(metrics *observability.Metrics, db *database.DB, version string, schemaVersion int64) fasthttp.RequestHandler {
	return func(ctx *fasthttp.RequestCtx) {
		var pool observability.PoolMetrics
		if db != nil {
			stats := db.PoolMetrics()
			pool = observability.PoolMetrics{
				OpenConnections: stats.OpenConnections,
				InUse:           stats.InUse,
				Idle:            stats.Idle,
				WaitCount:       stats.WaitCount,
				WaitDuration:    stats.WaitDuration,
			}
		}
		ctx.Response.Header.SetContentType("text/plain; version=0.0.4")
		ctx.SetStatusCode(fasthttp.StatusOK)
		ctx.SetBodyString(metrics.RenderPrometheus(pool, version, schemaVersion))
	}
}
