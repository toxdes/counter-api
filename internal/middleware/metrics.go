package middleware

import (
	"counter/internal/observability"

	"github.com/valyala/fasthttp"
)

type metricsContextKey struct{}

func MetricsContext(metrics *observability.Metrics) func(fasthttp.RequestHandler) fasthttp.RequestHandler {
	return func(next fasthttp.RequestHandler) fasthttp.RequestHandler {
		return func(ctx *fasthttp.RequestCtx) {
			ctx.SetUserValue(metricsContextKey{}, metrics)
			next(ctx)
		}
	}
}

func MetricsFromRequest(ctx *fasthttp.RequestCtx) *observability.Metrics {
	metrics, _ := ctx.UserValue(metricsContextKey{}).(*observability.Metrics)
	return metrics
}

func RecordEvent(ctx *fasthttp.RequestCtx, name string) {
	if metrics := MetricsFromRequest(ctx); metrics != nil {
		metrics.RecordEvent(name)
	}
}
