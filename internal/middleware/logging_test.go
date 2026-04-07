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
