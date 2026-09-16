package store

import (
	"context"
	"counter/internal/database"
	"counter/internal/models"
	"crypto/subtle"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"math/big"
	"time"

	"github.com/jmoiron/sqlx"
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
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin create counter: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	_, err = tx.ExecContext(ctx,
		"INSERT INTO counters (id, tenant_id, label, value, max_delta, created_at, updated_at) VALUES ($1, $2, $3, $4, $5, $6, $7)",
		counter.ID, counter.TenantID, counter.Label, counter.Value, counter.MaxDelta, counter.CreatedAt, counter.UpdatedAt,
	)
	if err != nil {
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

	_, err = tx.ExecContext(ctx, `
		INSERT INTO counter_operations (
			tenant_id, counter_id, operation_id, kind, delta, request_hash,
			value_before, value_after, state, created_at, completed_at
		)
		SELECT $1, $2, md5('counter-initial:' || $2::text)::uuid, 'initial_value', $3,
			decode(md5('counter-initial:' || $2::text), 'hex'), 0, $3, 'completed', $4, $4
	`, counter.TenantID, counter.ID, counter.Value, counter.CreatedAt)
	if err != nil {
		return fmt.Errorf("record initial counter value: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit create counter: %w", err)
	}
	committed = true
	return nil
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

type MutationResult struct {
	OperationID string
	Delta       int64
	Value       int64
	UpdatedAt   time.Time
	Replayed    bool
}

type operationRecord struct {
	Kind        string        `db:"kind"`
	CounterID   string        `db:"counter_id"`
	Delta       int64         `db:"delta"`
	RequestHash []byte        `db:"request_hash"`
	State       string        `db:"state"`
	ValueAfter  sql.NullInt64 `db:"value_after"`
	CompletedAt sql.NullTime  `db:"completed_at"`
}

func (s *CounterStore) IncrementCounter(ctx context.Context, tenantID, counterID string, delta int64, operationID string, requestHash []byte, now time.Time) (MutationResult, error) {
	return s.incrementCounter(ctx, tenantID, counterID, delta, operationID, requestHash, "", now)
}

func (s *CounterStore) IncrementCounterWithActor(ctx context.Context, tenantID, counterID string, delta int64, operationID string, requestHash []byte, actorID string, now time.Time) (MutationResult, error) {
	return s.incrementCounter(ctx, tenantID, counterID, delta, operationID, requestHash, actorID, now)
}

func (s *CounterStore) incrementCounter(ctx context.Context, tenantID, counterID string, delta int64, operationID string, requestHash []byte, actorID string, now time.Time) (MutationResult, error) {
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return MutationResult{}, fmt.Errorf("begin increment counter: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	var insertedID string
	err = tx.QueryRowxContext(ctx, `
		INSERT INTO counter_operations (
			tenant_id, counter_id, operation_id, kind, delta, request_hash, actor_id, state, created_at
		)
		VALUES ($1, $2, $3, 'increment', $4, $5, $6, 'pending', $7)
		ON CONFLICT (tenant_id, operation_id) DO NOTHING
		RETURNING operation_id
	`, tenantID, counterID, operationID, delta, requestHash, actorID, now).Scan(&insertedID)
	if errors.Is(err, sql.ErrNoRows) {
		result, resolveErr := resolveExistingOperation(ctx, tx, tenantID, operationID, counterID, "increment", &delta, requestHash)
		if resolveErr != nil {
			return MutationResult{}, resolveErr
		}
		if err := tx.Commit(); err != nil {
			return MutationResult{}, fmt.Errorf("commit increment replay: %w", err)
		}
		committed = true
		result.OperationID = operationID
		result.Replayed = true
		return result, nil
	}
	if err != nil {
		var pqErr *pq.Error
		if errors.As(err, &pqErr) && pqErr.Code == "23503" {
			return MutationResult{}, ErrCounterNotFound
		}
		return MutationResult{}, fmt.Errorf("reserve increment operation: %w", err)
	}

	var current struct {
		Value    int64 `db:"value"`
		MaxDelta int64 `db:"max_delta"`
	}
	if err := tx.GetContext(ctx, &current, `
		SELECT value, max_delta
		FROM counters
		WHERE tenant_id = $1 AND id = $2
		FOR UPDATE
	`, tenantID, counterID); errors.Is(err, sql.ErrNoRows) {
		return MutationResult{}, ErrCounterNotFound
	} else if err != nil {
		return MutationResult{}, fmt.Errorf("lock counter for increment: %w", err)
	}
	if delta <= 0 {
		return MutationResult{}, fmt.Errorf("increment counter: delta must be positive")
	}
	if delta > current.MaxDelta {
		return MutationResult{}, ErrDeltaExceedsMaximum
	}
	if current.Value > math.MaxInt64-delta {
		return MutationResult{}, ErrCounterOverflow
	}
	after := current.Value + delta

	if _, err := tx.ExecContext(ctx, `
		UPDATE counters
		SET value = $1, updated_at = $2
		WHERE tenant_id = $3 AND id = $4
	`, after, now, tenantID, counterID); err != nil {
		return MutationResult{}, fmt.Errorf("update counter for increment: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE counter_operations
		SET value_before = $1, value_after = $2, state = 'completed', completed_at = $3
		WHERE tenant_id = $4 AND operation_id = $5
	`, current.Value, after, now, tenantID, operationID); err != nil {
		return MutationResult{}, fmt.Errorf("complete increment operation: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return MutationResult{}, fmt.Errorf("commit increment counter: %w", err)
	}
	committed = true
	return MutationResult{OperationID: insertedID, Delta: delta, Value: after, UpdatedAt: now}, nil
}

func (s *CounterStore) SetCounterValueWithOperation(ctx context.Context, tenantID, counterID string, value int64, operationID string, requestHash []byte, now time.Time) (MutationResult, error) {
	return s.setCounterValueWithOperation(ctx, tenantID, counterID, value, operationID, requestHash, "", now)
}

func (s *CounterStore) SetCounterValueWithOperationWithActor(ctx context.Context, tenantID, counterID string, value int64, operationID string, requestHash []byte, actorID string, now time.Time) (MutationResult, error) {
	return s.setCounterValueWithOperation(ctx, tenantID, counterID, value, operationID, requestHash, actorID, now)
}

func (s *CounterStore) setCounterValueWithOperation(ctx context.Context, tenantID, counterID string, value int64, operationID string, requestHash []byte, actorID string, now time.Time) (MutationResult, error) {
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return MutationResult{}, fmt.Errorf("begin set counter: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	var current int64
	if err := tx.GetContext(ctx, &current, `
		SELECT value FROM counters
		WHERE tenant_id = $1 AND id = $2
		FOR UPDATE
	`, tenantID, counterID); errors.Is(err, sql.ErrNoRows) {
		return MutationResult{}, ErrCounterNotFound
	} else if err != nil {
		return MutationResult{}, fmt.Errorf("lock counter for set: %w", err)
	}

	deltaBig := new(big.Int).Sub(big.NewInt(value), big.NewInt(current))
	if !deltaBig.IsInt64() {
		return MutationResult{}, ErrCounterOverflow
	}
	delta := deltaBig.Int64()

	var insertedID string
	err = tx.QueryRowxContext(ctx, `
		INSERT INTO counter_operations (
			tenant_id, counter_id, operation_id, kind, delta, request_hash, actor_id, state, created_at
		)
		VALUES ($1, $2, $3, 'set_adjustment', $4, $5, $6, 'pending', $7)
		ON CONFLICT (tenant_id, operation_id) DO NOTHING
		RETURNING operation_id
	`, tenantID, counterID, operationID, delta, requestHash, actorID, now).Scan(&insertedID)
	if errors.Is(err, sql.ErrNoRows) {
		result, resolveErr := resolveExistingOperation(ctx, tx, tenantID, operationID, counterID, "set_adjustment", nil, requestHash)
		if resolveErr != nil {
			return MutationResult{}, resolveErr
		}
		if err := tx.Commit(); err != nil {
			return MutationResult{}, fmt.Errorf("commit set replay: %w", err)
		}
		committed = true
		result.OperationID = operationID
		result.Replayed = true
		return result, nil
	}
	if err != nil {
		return MutationResult{}, fmt.Errorf("reserve set operation: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `
		UPDATE counters
		SET value = $1, updated_at = $2
		WHERE tenant_id = $3 AND id = $4
	`, value, now, tenantID, counterID); err != nil {
		return MutationResult{}, fmt.Errorf("update counter for set: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE counter_operations
		SET value_before = $1, value_after = $2, state = 'completed', completed_at = $3
		WHERE tenant_id = $4 AND operation_id = $5
	`, current, value, now, tenantID, operationID); err != nil {
		return MutationResult{}, fmt.Errorf("complete set operation: %w", err)
	}
	if actorID != "" {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO auth_audit_events (
				id, tenant_id, actor_id, action, metadata, created_at
			) VALUES ($1, $2, $3, 'privileged_adjustment', $4, $5)
		`, operationID, tenantID, actorID, fmt.Sprintf(`{"operation_id":%q}`, operationID), now); err != nil {
			return MutationResult{}, fmt.Errorf("audit set operation: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return MutationResult{}, fmt.Errorf("commit set counter: %w", err)
	}
	committed = true
	return MutationResult{OperationID: insertedID, Delta: delta, Value: value, UpdatedAt: now}, nil
}

func resolveExistingOperation(ctx context.Context, tx *sqlx.Tx, tenantID, operationID, counterID, kind string, expectedDelta *int64, requestHash []byte) (MutationResult, error) {
	var existing operationRecord
	if err := tx.GetContext(ctx, &existing, `
		SELECT kind, counter_id, delta, request_hash, state, value_after, completed_at
		FROM counter_operations
		WHERE tenant_id = $1 AND operation_id = $2
	`, tenantID, operationID); errors.Is(err, sql.ErrNoRows) {
		return MutationResult{}, fmt.Errorf("resolve operation %s: %w", operationID, ErrIdempotencyKeyReused)
	} else if err != nil {
		return MutationResult{}, fmt.Errorf("load existing operation: %w", err)
	}

	if existing.Kind != kind || existing.CounterID != counterID || (expectedDelta != nil && existing.Delta != *expectedDelta) || subtle.ConstantTimeCompare(existing.RequestHash, requestHash) != 1 {
		return MutationResult{}, ErrIdempotencyKeyReused
	}
	if existing.State != "completed" {
		return MutationResult{}, ErrOperationInProgress
	}
	if !existing.ValueAfter.Valid || !existing.CompletedAt.Valid {
		return MutationResult{}, fmt.Errorf("completed operation %s has incomplete result", operationID)
	}
	return MutationResult{Delta: existing.Delta, Value: existing.ValueAfter.Int64, UpdatedAt: existing.CompletedAt.Time}, nil
}
