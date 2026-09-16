package store

import (
	"context"
	"counter/internal/database"
	"counter/internal/models"
	"database/sql"
	"errors"
	"fmt"

	"github.com/lib/pq"
)

// CounterStore contains PostgreSQL persistence for counter reads and creation.
type CounterStore struct {
	db *database.DB
}

func NewCounterStore(db *database.DB) *CounterStore {
	return &CounterStore{db: db}
}

func (s *CounterStore) TenantExists(ctx context.Context, tenantID string) (bool, error) {
	var exists bool
	if err := s.db.GetContext(ctx, &exists, "SELECT EXISTS(SELECT 1 FROM tenants WHERE id = $1)", tenantID); err != nil {
		return false, fmt.Errorf("check tenant: %w", err)
	}
	return exists, nil
}

func (s *CounterStore) CreateCounter(ctx context.Context, counter *models.Counter) error {
	_, err := s.db.ExecContext(ctx,
		"INSERT INTO counters (id, tenant_id, label, value, max_delta, created_at, updated_at) VALUES ($1, $2, $3, $4, $5, $6, $7)",
		counter.ID, counter.TenantID, counter.Label, counter.Value, counter.MaxDelta, counter.CreatedAt, counter.UpdatedAt,
	)
	if err == nil {
		return nil
	}

	var pqErr *pq.Error
	if errors.As(err, &pqErr) {
		switch pqErr.Code {
		case "23503":
			return ErrTenantNotFound
		case "23505":
			return ErrCounterConflict
		}
	}
	return fmt.Errorf("create counter: %w", err)
}

func (s *CounterStore) GetCounter(ctx context.Context, tenantID, counterID string) (*models.Counter, error) {
	var counter models.Counter
	err := s.db.GetContext(ctx, &counter,
		"SELECT id, tenant_id, label, value, max_delta, created_at, updated_at FROM counters WHERE id = $1 AND tenant_id = $2",
		counterID, tenantID,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrCounterNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get counter: %w", err)
	}
	return &counter, nil
}

func (s *CounterStore) ListCounters(ctx context.Context, tenantID string, cursor *models.CounterCursor, limit int) ([]models.Counter, error) {
	var counters []models.Counter
	var err error
	if cursor != nil {
		err = s.db.SelectContext(ctx, &counters, `
			SELECT id, tenant_id, label, value, max_delta, created_at, updated_at
			FROM counters
			WHERE tenant_id = $1 AND (created_at, id) > ($2, $3)
			ORDER BY created_at, id
			LIMIT $4
		`, tenantID, cursor.CreatedAt, cursor.ID, limit+1)
	} else {
		err = s.db.SelectContext(ctx, &counters, `
			SELECT id, tenant_id, label, value, max_delta, created_at, updated_at
			FROM counters
			WHERE tenant_id = $1
			ORDER BY created_at, id
			LIMIT $2
		`, tenantID, limit+1)
	}
	if err != nil {
		return nil, fmt.Errorf("list counters: %w", err)
	}
	return counters, nil
}
