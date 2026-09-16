package middleware

import (
	"strings"
	"testing"

	"github.com/valyala/fasthttp"
)

func TestRecoverKeepsProcessUsableAfterPanic(t *testing.T) {
	calls := 0
	handler := Recover(func(ctx *fasthttp.RequestCtx) {
		calls++
		if calls == 1 {
			panic("test panic")
		}
		ctx.SetStatusCode(fasthttp.StatusNoContent)
	})

	first := &fasthttp.RequestCtx{}
	handler(first)
	if first.Response.StatusCode() != fasthttp.StatusInternalServerError {
		t.Fatalf("first status = %d, want %d", first.Response.StatusCode(), fasthttp.StatusInternalServerError)
	}
	second := &fasthttp.RequestCtx{}
	handler(second)
	if second.Response.StatusCode() != fasthttp.StatusNoContent {
		t.Fatalf("second status = %d, want %d", second.Response.StatusCode(), fasthttp.StatusNoContent)
	}
}

func TestRouteTemplateBoundsUUIDCardinality(t *testing.T) {
	path := "/v2/tenants/123e4567-e89b-12d3-a456-426614174000/counters/123e4567-e89b-12d3-a456-426614174001/inc"
	if got := RouteTemplate(path); got != "/v2/tenants/:id/counters/:id/inc" {
		t.Fatalf("RouteTemplate() = %q", got)
	}
}

func TestRedactSensitiveStackRemovesRequestSecrets(t *testing.T) {
	ctx := &fasthttp.RequestCtx{}
	ctx.Request.Header.Set("X-API-Key", "secret-key")
	stack := redactSensitiveStack(ctx, "panic: secret-key\nstack")
	if strings.Contains(stack, "secret-key") || !strings.Contains(stack, "[REDACTED]") {
		t.Fatalf("redacted stack = %q", stack)
	}
}
