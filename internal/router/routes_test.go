package router

import (
	"counter/internal/middleware"
	"strings"
	"testing"

	"github.com/valyala/fasthttp"
)

func TestSharedRouteSurfaceIsComplete(t *testing.T) {
	want := []string{
		"GET /",
		"GET /tools/curl",
		"GET /livez",
		"GET /readyz",
		"GET /metrics",
		"POST /v2/tenants",
		"GET /v2/tenants/<tenant_id>",
		"GET /v2/tenants/<tenant_id>/counters",
		"POST /v2/tenants/<tenant_id>/counters",
		"POST /v2/tenants/<tenant_id>/counters/<counter_id>/inc",
		"POST /v2/tenants/<tenant_id>/counters/<counter_id>/set",
		"GET /v2/tenants/<tenant_id>/counters/<counter_id>",
		"GET /v2/tenants/<tenant_id>/counters/<counter_id>/operations",
		"GET /v2/tenants/<tenant_id>/counters/<counter_id>/metadata",
		"PUT /v2/tenants/<tenant_id>/counters/<counter_id>/metadata",
		"DELETE /v2/tenants/<tenant_id>/counters/<counter_id>/metadata",
		"POST /v2/tenants/<tenant_id>/credentials",
		"POST /v2/tenants/<tenant_id>/credentials/<credential_id>/rotate",
		"POST /v2/tenants/<tenant_id>/credentials/<credential_id>/revoke",
		"POST /v2/admin/credentials",
		"OPTIONS /*",
	}
	if got := sharedRouteSurface(); len(got) != len(want) {
		t.Fatalf("route surface has %d entries, want %d: %#v", len(got), len(want), got)
	}
	for index := range want {
		if got := sharedRouteSurface()[index]; got != want[index] {
			t.Fatalf("route %d = %q, want %q", index, got, want[index])
		}
	}
}

func TestRetiredV1RoutesDoNotServeDataOrMutate(t *testing.T) {
	// A nil database also proves these paths never reach a persistence handler.
	router := NewRouter(nil, &middleware.CORSConfig{AllowedOrigins: "*"}, middleware.NewRateLimiter(1000, 3, 60), "test-key", middleware.NewDefaultLogger("error"), nil)
	const tenant = "123e4567-e89b-12d3-a456-426614174000"
	const counter = "123e4567-e89b-12d3-a456-426614174001"
	for _, route := range []string{
		"POST /tenants",
		"GET /tenants/" + tenant,
		"GET /tenants/" + tenant + "/counters",
		"POST /tenants/" + tenant + "/counters",
		"GET /tenants/" + tenant + "/counters/" + counter,
		"POST /tenants/" + tenant + "/counters/" + counter + "/inc?delta=1",
		"POST /tenants/" + tenant + "/counters/" + counter + "/set",
	} {
		for _, key := range []string{"", "test-key"} {
			t.Run(route+"/"+key, func(t *testing.T) {
				parts := strings.SplitN(route, " ", 2)
				ctx := &fasthttp.RequestCtx{}
				ctx.Request.Header.SetMethod(parts[0])
				ctx.Request.SetRequestURI(parts[1])
				ctx.Request.Header.Set("X-API-Key", key)
				ctx.Request.SetBodyString(`{"label":"retired","value":100}`)
				router.ServeHTTP(ctx)
				if ctx.Response.StatusCode() != fasthttp.StatusNotFound {
					t.Fatalf("retired route returned %d: %s", ctx.Response.StatusCode(), ctx.Response.Body())
				}
			})
		}
	}
}

func TestEveryV2DataRouteRequiresAuthentication(t *testing.T) {
	router := NewRouter(nil, &middleware.CORSConfig{AllowedOrigins: "*"}, middleware.NewRateLimiter(1000, 3, 60), "test-key", middleware.NewDefaultLogger("error"), nil)
	for _, route := range sharedRouteSurface() {
		parts := strings.SplitN(route, " ", 2)
		if !strings.HasPrefix(parts[1], "/v2/") {
			continue
		}
		t.Run(route, func(t *testing.T) {
			ctx := &fasthttp.RequestCtx{}
			ctx.Request.Header.SetMethod(parts[0])
			path := strings.ReplaceAll(parts[1], "<tenant_id>", "123e4567-e89b-12d3-a456-426614174000")
			path = strings.ReplaceAll(path, "<counter_id>", "123e4567-e89b-12d3-a456-426614174001")
			path = strings.ReplaceAll(path, "<credential_id>", "123e4567-e89b-12d3-a456-426614174002")
			ctx.Request.SetRequestURI(path)
			router.ServeHTTP(ctx)
			if ctx.Response.StatusCode() != fasthttp.StatusUnauthorized {
				t.Fatalf("data route returned %d without credentials: %s", ctx.Response.StatusCode(), ctx.Response.Body())
			}
		})
	}
}
