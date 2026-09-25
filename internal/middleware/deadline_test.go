package middleware

import (
	"context"
	"testing"
	"time"

	"github.com/valyala/fasthttp"
)

func TestRequestContextUsesRouteClassDeadline(t *testing.T) {
	readTimeout := 2 * time.Second
	mutationTimeout := 7 * time.Second
	var gotDeadline time.Time

	handler := RequestContext(RouteTimeouts{
		Read:     readTimeout,
		Mutation: mutationTimeout,
	})(func(ctx *fasthttp.RequestCtx) {
		requestContext, ok := RequestContextFromRequest(ctx)
		if !ok {
			t.Fatal("request context was not attached")
		}
		gotDeadline, _ = requestContext.Deadline()
	})

	ctx := &fasthttp.RequestCtx{}
	ctx.Request.Header.SetMethod(fasthttp.MethodPost)
	ctx.Request.SetRequestURI("/v2/tenants/tenant/counters/counter/inc")
	started := time.Now()
	handler(ctx)

	if gotDeadline.Before(started.Add(mutationTimeout - 100*time.Millisecond)) {
		t.Fatalf("deadline = %s, expected at least %s from start", gotDeadline, started.Add(mutationTimeout-100*time.Millisecond))
	}
	if gotDeadline.After(started.Add(mutationTimeout + 100*time.Millisecond)) {
		t.Fatalf("deadline = %s, expected at most %s from start", gotDeadline, started.Add(mutationTimeout+100*time.Millisecond))
	}
}

func TestDatabaseContextBudgetIsShorterThanRequestDeadline(t *testing.T) {
	requestContext, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	databaseContext, databaseCancel := DatabaseContext(requestContext, 500*time.Millisecond)
	defer databaseCancel()

	requestDeadline, _ := requestContext.Deadline()
	databaseDeadline, _ := databaseContext.Deadline()
	if !databaseDeadline.Before(requestDeadline) {
		t.Fatalf("database deadline %s should precede request deadline %s", databaseDeadline, requestDeadline)
	}
}

func TestConcurrencyLimitReturnsServiceUnavailableWhenSaturated(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	handler := ConcurrencyLimit(1)(func(ctx *fasthttp.RequestCtx) {
		close(entered)
		<-release
		ctx.SetStatusCode(fasthttp.StatusOK)
	})

	first := &fasthttp.RequestCtx{}
	done := make(chan struct{})
	go func() {
		handler(first)
		close(done)
	}()
	<-entered

	second := &fasthttp.RequestCtx{}
	handler(second)
	if second.Response.StatusCode() != fasthttp.StatusServiceUnavailable {
		t.Fatalf("saturated request status = %d, want %d", second.Response.StatusCode(), fasthttp.StatusServiceUnavailable)
	}
	if string(second.Response.Header.Peek("Retry-After")) != "1" {
		t.Fatalf("Retry-After = %q, want 1", second.Response.Header.Peek("Retry-After"))
	}

	close(release)
	<-done
}
