package observability

import (
	"context"
	"errors"
	"testing"
)

func TestHealthStateReadinessRequiresStartupSchemaAndDependency(t *testing.T) {
	state := NewHealthState(7)

	if err := state.CheckReady(context.Background(), func(context.Context) error { return nil }); err == nil {
		t.Fatal("unstarted process should not be ready")
	}

	state.MarkStarted(6)
	if err := state.CheckReady(context.Background(), func(context.Context) error { return nil }); err == nil {
		t.Fatal("schema-incompatible process should not be ready")
	}

	state.MarkStarted(7)
	dependencyErr := errors.New("database unavailable")
	if err := state.CheckReady(context.Background(), func(context.Context) error { return dependencyErr }); !errors.Is(err, dependencyErr) {
		t.Fatalf("dependency error = %v, want %v", err, dependencyErr)
	}

	if err := state.CheckReady(context.Background(), func(context.Context) error { return nil }); err != nil {
		t.Fatalf("ready state returned error: %v", err)
	}

	state.MarkDraining()
	if err := state.CheckReady(context.Background(), func(context.Context) error { return nil }); err == nil {
		t.Fatal("draining process should not be ready")
	}
}
