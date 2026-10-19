package router

import (
	"context"
	"counter/internal/middleware"
	"counter/internal/service"
	"counter/internal/store"
	"counter/internal/testutil"
	"testing"

	"github.com/google/uuid"
	"github.com/valyala/fasthttp"
)

func TestV2RoutesEnforceActionScopesAndTenantOwnership(t *testing.T) {
	db := testutil.OpenPostgres(t)
	router := idempotencyReplica(db)
	tenantID := createTestTenant(t, db, "v2-authorization")
	otherTenantID := createTestTenant(t, db, "other-v2-authorization")
	counterID := createTestCounter(t, db, tenantID, "protected", 7)
	credentials := service.NewCredentialService(store.NewCredentialStore(db))
	issue := func(tenantID string, scopes []string) string {
		t.Helper()
		credential, err := credentials.Create(context.Background(), tenantID, scopes, nil, "test-admin")
		if err != nil {
			t.Fatal(err)
		}
		return credential.APIKey
	}
	base := "/v2/tenants/" + tenantID
	for _, tc := range []struct {
		name, method, path, body, scope string
		status                          int
	}{
		{"tenant read", "GET", base, "", middleware.ScopeTenantRead, 200},
		{"counter list", "GET", base + "/counters", "", middleware.ScopeCounterList, 200},
		{"counter create", "POST", base + "/counters", `{"label":"scoped-create"}`, middleware.ScopeCounterCreate, 201},
		{"counter read", "GET", base + "/counters/" + counterID, "", middleware.ScopeCounterRead, 200},
		{"increment", "POST", base + "/counters/" + counterID + "/inc", "", middleware.ScopeCounterIncrement, 200},
		{"set", "POST", base + "/counters/" + counterID + "/set", `{"value":100}`, middleware.ScopeCounterAdjust, 200},
		{"history", "GET", base + "/counters/" + counterID + "/operations", "", middleware.ScopeCounterHistory, 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			allowed := issue(tenantID, []string{tc.scope})
			other := issue(otherTenantID, []string{tc.scope})
			wrongScope := middleware.ScopeTenantRead
			if tc.scope == wrongScope {
				wrongScope = middleware.ScopeCounterRead
			}
			unprivileged := issue(tenantID, []string{wrongScope})
			request := func(key, operationID string) int {
				ctx := &fasthttp.RequestCtx{}
				ctx.Request.Header.SetMethod(tc.method)
				ctx.Request.SetRequestURI(tc.path)
				ctx.Request.SetBodyString(tc.body)
				ctx.Request.Header.Set("X-API-Key", key)
				ctx.Request.Header.Set("Idempotency-Key", operationID)
				router.ServeHTTP(ctx)
				return ctx.Response.StatusCode()
			}
			for _, denied := range []struct {
				key    string
				status int
			}{{"", 401}, {unprivileged, 403}, {other, 403}} {
				if status := request(denied.key, uuid.NewString()); status != denied.status {
					t.Fatalf("unauthorized request status = %d, want %d", status, denied.status)
				}
			}
			if tc.scope == middleware.ScopeCounterIncrement || tc.scope == middleware.ScopeCounterAdjust {
				if status := request(allowed, ""); status != 400 {
					t.Fatalf("mutation without an idempotency key returned %d", status)
				}
			}
			if status := request(allowed, uuid.NewString()); status != tc.status {
				t.Fatalf("authorized request status = %d, want %d", status, tc.status)
			}
		})
	}
	// Rejected writes must not affect the original counter or its history.
	var value, operations int
	if err := db.Get(&value, "SELECT value FROM counters WHERE id = $1", counterID); err != nil {
		t.Fatal(err)
	}
	if err := db.Get(&operations, "SELECT COUNT(*) FROM counter_operations WHERE counter_id = $1", counterID); err != nil {
		t.Fatal(err)
	}
	if value != 100 || operations != 2 {
		t.Fatalf("unexpected mutation effects: value=%d, operations=%d", value, operations)
	}
}
