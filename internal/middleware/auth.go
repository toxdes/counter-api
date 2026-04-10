package middleware

import (
	"github.com/qiangxue/fasthttp-routing"
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

// APIKeyAuthRouting returns an API key authentication middleware for routing library
func APIKeyAuthRouting(expectedKey string) func(routing.Handler) routing.Handler {
	return func(next routing.Handler) routing.Handler {
		return func(c *routing.Context) error {
			providedKey := string(c.RequestCtx.Request.Header.Peek("X-API-Key"))

			if providedKey != expectedKey {
				c.RequestCtx.SetStatusCode(fasthttp.StatusUnauthorized)
				c.RequestCtx.SetBodyString(`{"error":"UNAUTHORIZED","message":"Invalid or missing API key"}`)
				// Don't call next() to stop the chain
				return nil
			}

			return next(c)
		}
	}
}
