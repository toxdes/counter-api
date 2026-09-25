package observability

import (
	"context"
	"errors"
	"sync/atomic"
)

var (
	ErrNotStarted     = errors.New("service has not completed startup")
	ErrSchemaMismatch = errors.New("database schema is incompatible")
	ErrDraining       = errors.New("service is draining")
)

// HealthState tracks the lifecycle conditions used by readiness checks.
// Liveness intentionally remains true during draining so orchestrators do not
// restart a healthy process while it is handing off traffic.
type HealthState struct {
	supportedSchemaVersion int64
	started                atomic.Bool
	draining               atomic.Bool
	alive                  atomic.Bool
	schemaVersion          atomic.Int64
}

func NewHealthState(supportedSchemaVersion int64) *HealthState {
	state := &HealthState{supportedSchemaVersion: supportedSchemaVersion}
	state.alive.Store(true)
	return state
}

func (s *HealthState) MarkStarted(schemaVersion int64) {
	s.schemaVersion.Store(schemaVersion)
	s.started.Store(true)
	s.alive.Store(true)
	s.draining.Store(false)
}

func (s *HealthState) MarkDraining() {
	s.draining.Store(true)
}

func (s *HealthState) MarkStopped() {
	s.started.Store(false)
	s.alive.Store(false)
}

func (s *HealthState) Live() bool {
	return s.alive.Load()
}

func (s *HealthState) CheckReady(ctx context.Context, ping func(context.Context) error) error {
	if !s.started.Load() {
		return ErrNotStarted
	}
	if s.draining.Load() {
		return ErrDraining
	}
	if s.schemaVersion.Load() != s.supportedSchemaVersion {
		return ErrSchemaMismatch
	}
	if ping == nil {
		return errors.New("readiness dependency is not configured")
	}
	return ping(ctx)
}
