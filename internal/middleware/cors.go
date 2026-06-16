package middleware

import (
	"strconv"
	"strings"

	"github.com/valyala/fasthttp"
)

// CORSConfig holds CORS configuration
type CORSConfig struct {
	AllowedOrigins   string
	AllowedMethods   string
	AllowedHeaders   string
	AllowCredentials bool
	MaxAge           int
}

// CORS returns a CORS middleware handler
func CORS(config *CORSConfig) func(fasthttp.RequestHandler) fasthttp.RequestHandler {
	return func(next fasthttp.RequestHandler) fasthttp.RequestHandler {
		return func(ctx *fasthttp.RequestCtx) {
			origin := string(ctx.Request.Header.Peek("Origin"))

			if origin != "" {
				if isOriginAllowed(origin, config.AllowedOrigins) {
					ctx.Response.Header.Set("Access-Control-Allow-Origin", origin)
				}
			}

			ctx.Response.Header.Set("Access-Control-Allow-Methods", config.AllowedMethods)
			ctx.Response.Header.Set("Access-Control-Allow-Headers", config.AllowedHeaders)

			if config.AllowCredentials {
				ctx.Response.Header.Set("Access-Control-Allow-Credentials", "true")
			}

			if config.MaxAge > 0 {
				ctx.Response.Header.Set("Access-Control-Max-Age", strconv.Itoa(config.MaxAge))
			}

			// Handle preflight requests
			if string(ctx.Method()) == "OPTIONS" {
				ctx.SetStatusCode(fasthttp.StatusOK)
				return
			}

			next(ctx)
		}
	}
}

// isOriginAllowed checks if the origin is in the allowed origins list
func isOriginAllowed(origin, allowedOrigins string) bool {
	if allowedOrigins == "*" {
		return true
	}

	for _, allowed := range strings.Split(allowedOrigins, ",") {
		if strings.TrimSpace(allowed) == origin {
			return true
		}
	}
	return false
}
