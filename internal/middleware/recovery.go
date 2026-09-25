package middleware

import (
	"log"
	"runtime/debug"
	"strings"

	"github.com/google/uuid"
	"github.com/valyala/fasthttp"
)

// Recover turns route panics into generic server errors and keeps the process
// alive. The stack is logged locally but never returned to the caller.
func Recover(next fasthttp.RequestHandler) fasthttp.RequestHandler {
	return func(ctx *fasthttp.RequestCtx) {
		defer func() {
			if recovered := recover(); recovered != nil {
				log.Printf("panic recovered:\n%s", redactSensitiveStack(ctx, string(debug.Stack())))
				ctx.SetStatusCode(fasthttp.StatusInternalServerError)
				ctx.Response.Header.SetContentType("application/json")
				ctx.SetBodyString(`{"errors":[{"code":"INTERNAL_SERVER_ERROR","message":"Internal server error"}]}`)
			}
		}()
		next(ctx)
	}
}

func redactSensitiveStack(ctx *fasthttp.RequestCtx, stack string) string {
	for _, header := range []string{"X-API-Key", "Authorization", "Cookie"} {
		if value := string(ctx.Request.Header.Peek(header)); value != "" {
			stack = strings.ReplaceAll(stack, value, "[REDACTED]")
		}
	}
	return stack
}

// RouteTemplate bounds telemetry cardinality by replacing UUID path segments
// with a stable parameter marker.
func RouteTemplate(path string) string {
	staticSegments := map[string]struct{}{
		"v2": {}, "tenants": {}, "counters": {}, "inc": {}, "set": {},
		"operations": {}, "credentials": {}, "rotate": {}, "revoke": {},
		"admin": {}, "livez": {}, "readyz": {}, "metrics": {},
	}
	parts := strings.Split(path, "/")
	for index, part := range parts {
		if part == "" {
			continue
		}
		if _, err := uuid.Parse(part); err == nil {
			parts[index] = ":id"
			continue
		}
		if _, ok := staticSegments[part]; !ok {
			parts[index] = ":param"
		}
	}
	return strings.Join(parts, "/")
}
