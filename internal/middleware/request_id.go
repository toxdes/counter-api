package middleware

import (
	"strings"

	"github.com/google/uuid"
	"github.com/valyala/fasthttp"
)

type requestIDKey struct{}

// RequestIDFromRequest returns the request ID assigned by the outer
// middleware, if any.
func RequestIDFromRequest(ctx *fasthttp.RequestCtx) string {
	requestID, _ := ctx.UserValue(requestIDKey{}).(string)
	return requestID
}

func setRequestID(ctx *fasthttp.RequestCtx, requestID string) {
	ctx.SetUserValue(requestIDKey{}, requestID)
	ctx.Response.Header.Set("X-Request-ID", requestID)
}

func requestIDFromHeader(ctx *fasthttp.RequestCtx) string {
	requestID := strings.TrimSpace(string(ctx.Request.Header.Peek("X-Request-ID")))
	if len(requestID) > 128 || strings.ContainsAny(requestID, "\r\n") {
		return ""
	}
	return requestID
}

func ensureRequestID(ctx *fasthttp.RequestCtx) string {
	if requestID := RequestIDFromRequest(ctx); requestID != "" {
		return requestID
	}
	requestID := requestIDFromHeader(ctx)
	if requestID == "" {
		requestID = uuid.NewString()
	}
	setRequestID(ctx, requestID)
	return requestID
}
