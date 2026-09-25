package handlers

import (
	"context"
	"counter/internal/observability"
	"testing"

	"github.com/valyala/fasthttp"
)

type unavailableDatabase struct{}

func (unavailableDatabase) PingContext(context.Context) error {
	return context.DeadlineExceeded
}

func TestLivenessAndReadinessHandlers(t *testing.T) {
	state := observability.NewHealthState(7)
	live := LivenessHandler(state)
	ready := ReadinessHandler(state, nil)

	liveContext := &fasthttp.RequestCtx{}
	liveContext.Request.SetRequestURI("/livez")
	live(liveContext)
	if liveContext.Response.StatusCode() != fasthttp.StatusOK {
		t.Fatalf("liveness status = %d, want %d", liveContext.Response.StatusCode(), fasthttp.StatusOK)
	}

	readyContext := &fasthttp.RequestCtx{}
	readyContext.Request.SetRequestURI("/readyz")
	ready(readyContext)
	if readyContext.Response.StatusCode() != fasthttp.StatusServiceUnavailable {
		t.Fatalf("unstarted readiness status = %d, want %d", readyContext.Response.StatusCode(), fasthttp.StatusServiceUnavailable)
	}
}

func TestReadinessRemovesReplicaWhenWriterIsUnavailable(t *testing.T) {
	state := observability.NewHealthState(7)
	state.MarkStarted(7)
	ready := ReadinessHandler(state, unavailableDatabase{})

	ctx := &fasthttp.RequestCtx{}
	ctx.Request.SetRequestURI("/readyz")
	ready(ctx)

	if ctx.Response.StatusCode() != fasthttp.StatusServiceUnavailable {
		t.Fatalf("writer outage readiness status = %d, want %d", ctx.Response.StatusCode(), fasthttp.StatusServiceUnavailable)
	}
}

func TestMetricsHandlerReturnsPrometheusContent(t *testing.T) {
	metrics := observability.NewMetrics()
	metrics.RecordEvent("service_error")
	ctx := &fasthttp.RequestCtx{}
	ctx.Request.SetRequestURI("/metrics")

	MetricsHandler(metrics, nil, "test", 7)(ctx)
	if ctx.Response.StatusCode() != fasthttp.StatusOK {
		t.Fatalf("metrics status = %d, want %d", ctx.Response.StatusCode(), fasthttp.StatusOK)
	}
	if got := string(ctx.Response.Header.ContentType()); got != "text/plain; version=0.0.4" {
		t.Fatalf("metrics content type = %q", got)
	}
}
