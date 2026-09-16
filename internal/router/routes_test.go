package router

import "testing"

func TestSharedRouteSurfaceIsComplete(t *testing.T) {
	want := []string{
		"GET /",
		"POST /tenants",
		"GET /tenants/<tenant_id>",
		"GET /tenants/<tenant_id>/counters",
		"POST /tenants/<tenant_id>/counters",
		"GET /tenants/<tenant_id>/counters/<counter_id>",
		"POST /tenants/<tenant_id>/counters/<counter_id>/inc",
		"POST /tenants/<tenant_id>/counters/<counter_id>/set",
		"POST /v2/tenants/<tenant_id>/counters/<counter_id>/inc",
		"POST /v2/tenants/<tenant_id>/counters/<counter_id>/set",
		"GET /v2/tenants/<tenant_id>/counters/<counter_id>/operations",
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
