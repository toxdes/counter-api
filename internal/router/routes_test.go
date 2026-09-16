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
