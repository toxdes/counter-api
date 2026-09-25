package middleware

import (
	"net"
	"strings"

	"github.com/valyala/fasthttp"
)

type clientIPContextKey struct{}

// ClientIdentity establishes one canonical client identity for downstream
// authentication, rate limiting, logs, and telemetry.
func ClientIdentity(next fasthttp.RequestHandler) fasthttp.RequestHandler {
	return func(ctx *fasthttp.RequestCtx) {
		ctx.SetUserValue(clientIPContextKey{}, CanonicalClientIP(ctx))
		next(ctx)
	}
}

func ClientIPFromRequest(ctx *fasthttp.RequestCtx) string {
	if ip, ok := ctx.UserValue(clientIPContextKey{}).(string); ok && ip != "" {
		return ip
	}
	return CanonicalClientIP(ctx)
}

// CanonicalClientIP trusts forwarding headers only from the local nginx peer.
// Cloudflare's address is therefore accepted only after nginx has forwarded it
// from the loopback connection to this process.
func CanonicalClientIP(ctx *fasthttp.RequestCtx) string {
	remoteIP := ctx.RemoteIP()
	if IsTrustedProxy(remoteIP) {
		if ip := net.ParseIP(strings.TrimSpace(string(ctx.Request.Header.Peek("X-Real-IP")))); ip != nil {
			return ip.String()
		}
		if forwarded := ctx.Request.Header.Peek("X-Forwarded-For"); len(forwarded) > 0 {
			for _, candidate := range strings.Split(string(forwarded), ",") {
				if ip := net.ParseIP(strings.TrimSpace(candidate)); ip != nil {
					return ip.String()
				}
			}
		}
	}
	if remoteIP == nil {
		return "unknown"
	}
	return remoteIP.String()
}

func IsTrustedProxy(ip net.IP) bool {
	return ip != nil && ip.IsLoopback()
}
