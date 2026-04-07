package middleware

import (
	"github.com/valyala/fasthttp"
)

// APIKeyAuth returns an API key authentication middleware
func APIKeyAuth(expectedKey string) func(fasthttp.RequestHandler) fasthttp.RequestHandler {
	return func(next fasthttp.RequestHandler) fasthttp.RequestHandler {
		return func(ctx *fasthttp.RequestCtx) {
			providedKey := string(ctx.Request.Header.Peek("X-API-Key"))

			if providedKey != expectedKey {
				ctx.SetStatusCode(fasthttp.StatusUnauthorized)
				return
			}

			next(ctx)
		}
	}
}
