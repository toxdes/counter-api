package middleware

import (
	"context"
	"time"

	"github.com/valyala/fasthttp"
)

const (
	defaultReadTimeout     = 5 * time.Second
	defaultMutationTimeout = 10 * time.Second
	defaultDatabaseTimeout = 2 * time.Second
)

// RouteTimeouts defines the budgets applied to one request. Database is kept
// shorter than the request budget so the server has time to return a useful
// response instead of letting database work consume the entire request.
type RouteTimeouts struct {
	Read     time.Duration
	Mutation time.Duration
	Database time.Duration
}

type requestContextKey struct{}

type requestContextState struct {
	request  context.Context
	database context.Context
}

// DefaultRouteTimeouts returns the safe defaults used by compatibility router
// constructors and local callers.
func DefaultRouteTimeouts() RouteTimeouts {
	return RouteTimeouts{
		Read:     defaultReadTimeout,
		Mutation: defaultMutationTimeout,
		Database: defaultDatabaseTimeout,
	}
}

// RequestContext creates explicit request and database deadlines for
// fasthttp. fasthttp does not expose a net/http Request.Context, so the
// contexts are attached to the request for handlers and services to consume.
func RequestContext(timeouts RouteTimeouts) func(fasthttp.RequestHandler) fasthttp.RequestHandler {
	defaults := DefaultRouteTimeouts()
	if timeouts.Read <= 0 {
		timeouts.Read = defaults.Read
	}
	if timeouts.Mutation <= 0 {
		timeouts.Mutation = defaults.Mutation
	}
	if timeouts.Database <= 0 {
		timeouts.Database = defaults.Database
	}

	return func(next fasthttp.RequestHandler) fasthttp.RequestHandler {
		return func(ctx *fasthttp.RequestCtx) {
			timeout := timeouts.Mutation
			method := string(ctx.Method())
			if method == fasthttp.MethodGet || method == fasthttp.MethodHead || method == fasthttp.MethodOptions {
				timeout = timeouts.Read
			}

			requestContext, requestCancel := context.WithTimeout(context.Background(), timeout)
			databaseContext, databaseCancel := DatabaseContext(requestContext, timeouts.Database)
			ctx.SetUserValue(requestContextKey{}, requestContextState{
				request:  requestContext,
				database: databaseContext,
			})
			defer requestCancel()
			defer databaseCancel()

			next(ctx)
		}
	}
}

// RequestContextFromRequest returns the explicit request context or a
// background context for handlers invoked directly by compatibility tests.
func RequestContextFromRequest(ctx *fasthttp.RequestCtx) (context.Context, bool) {
	state, ok := ctx.UserValue(requestContextKey{}).(requestContextState)
	if !ok || state.request == nil {
		return context.Background(), false
	}
	return state.request, true
}

// DatabaseContextFromRequest returns the shorter database budget associated
// with a request, falling back to a background context for direct handlers.
func DatabaseContextFromRequest(ctx *fasthttp.RequestCtx) (context.Context, bool) {
	state, ok := ctx.UserValue(requestContextKey{}).(requestContextState)
	if !ok || state.database == nil {
		return context.Background(), false
	}
	return state.database, true
}

// DatabaseContext derives a bounded database context from a request context.
// If the parent deadline is already sooner, it is preserved.
func DatabaseContext(parent context.Context, budget time.Duration) (context.Context, context.CancelFunc) {
	if budget <= 0 {
		return parent, func() {}
	}
	if deadline, ok := parent.Deadline(); ok && time.Until(deadline) <= budget {
		return parent, func() {}
	}
	return context.WithTimeout(parent, budget)
}
