package middleware

import (
	"bytes"
	"testing"

	"github.com/valyala/fasthttp"
)

func TestLoggingMiddleware(t *testing.T) {
	var buf bytes.Buffer

	handler := Logging(&buf)(func(ctx *fasthttp.RequestCtx) {
		ctx.SetStatusCode(fasthttp.StatusOK)
	})

	ctx := &fasthttp.RequestCtx{}
	ctx.Request.SetRequestURI("/test")
	ctx.Request.Header.SetMethod("GET")

	handler(ctx)

	// Check that request ID was set
	requestID := string(ctx.Response.Header.Peek("X-Request-ID"))
	if requestID == "" {
		t.Error("Expected X-Request-ID header to be set")
	}

	// Check that log was written
	logOutput := buf.String()
	if logOutput == "" {
		t.Error("Expected log output, got empty string")
	}
}

func TestLoggingMiddlewarePropagatesProvidedRequestID(t *testing.T) {
	var buf bytes.Buffer
	ctx := &fasthttp.RequestCtx{}
	ctx.Request.SetRequestURI("/test")
	ctx.Request.Header.SetMethod("GET")
	ctx.Request.Header.Set("X-Request-ID", "request-from-edge")

	Logging(&buf)(func(ctx *fasthttp.RequestCtx) {
		if got := RequestIDFromRequest(ctx); got != "request-from-edge" {
			t.Fatalf("request ID in handler = %q, want request-from-edge", got)
		}
		ctx.SetStatusCode(fasthttp.StatusOK)
	})(ctx)

	if got := string(ctx.Response.Header.Peek("X-Request-ID")); got != "request-from-edge" {
		t.Fatalf("response request ID = %q, want request-from-edge", got)
	}
	if !bytes.Contains(buf.Bytes(), []byte(`"request_id":"request-from-edge"`)) {
		t.Fatalf("log did not contain propagated request ID: %s", buf.Bytes())
	}
}
