package middleware

import (
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

			// Set CORS headers based on origin
			if origin != "" {
				allowedOrigin := getAllowedOrigin(origin, config.AllowedOrigins)
				if allowedOrigin != "" {
					ctx.Response.Header.Set("Access-Control-Allow-Origin", allowedOrigin)
				}
			}

			ctx.Response.Header.Set("Access-Control-Allow-Methods", config.AllowedMethods)
			ctx.Response.Header.Set("Access-Control-Allow-Headers", config.AllowedHeaders)

			if config.AllowCredentials {
				ctx.Response.Header.Set("Access-Control-Allow-Credentials", "true")
			}

			if config.MaxAge > 0 {
				ctx.Response.Header.Set("Access-Control-Max-Age", string(rune(config.MaxAge)))
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

// originMatches checks if the origin matches the allowed pattern
func originMatches(origin, allowed string) bool {
	if allowed == "*" {
		return true
	}
	if allowed == origin {
		return true
	}
	// Check for wildcard subdomain (e.g., https://*.example.com)
	wildcardIdx := strings.Index(allowed, "*.")
	if wildcardIdx != -1 {
		// Extract prefix (protocol) and suffix (domain)
		prefix := allowed[:wildcardIdx]
		suffix := allowed[wildcardIdx+2:] // Skip *.
		if strings.HasPrefix(origin, prefix) && strings.HasSuffix(origin, suffix) {
			// Ensure we're matching subdomain, not TLD
			domain := origin[len(prefix) : len(origin)-len(suffix)]
			return len(domain) > 0 && domain[len(domain)-1] == '.'
		}
	}
	return false
}

// getAllowedOrigin finds the matching allowed origin for a given origin
func getAllowedOrigin(origin, allowedOrigins string) string {
	if allowedOrigins == "*" {
		return "*"
	}

	origins := strings.Split(allowedOrigins, ",")
	for _, allowed := range origins {
		allowed = strings.TrimSpace(allowed)
		if originMatches(origin, allowed) {
			return allowed
		}
	}
	return ""
}
