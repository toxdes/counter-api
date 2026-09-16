package store

import (
	"context"
	"counter/internal/models"
	"database/sql"
	"errors"
	"fmt"
	"math/big"
	"time"
)

type operationRow struct {
	OperationID string     `db:"operation_id"`
	CounterID   string     `db:"counter_id"`
	Kind        string     `db:"kind"`
	Delta       int64      `db:"delta"`
	ValueBefore *int64     `db:"value_before"`
	ValueAfter  *int64     `db:"value_after"`
	ActorID     *string    `db:"actor_id"`
	Metadata    []byte     `db:"metadata"`
	CreatedAt   time.Time  `db:"created_at"`
	CompletedAt *time.Time `db:"completed_at"`
}

func (s *CounterStore) ListOperations(ctx context.Context, tenantID, counterID string, cursor *models.OperationCursor, limit int) ([]models.CounterOperation, error) {
	var rows []operationRow
	var err error
	if cursor == nil {
		err = s.db.SelectContext(ctx, &rows, `
			SELECT operation_id, counter_id, kind, delta, value_before, value_after,
				actor_id, metadata, created_at, completed_at
			FROM counter_operations
			WHERE tenant_id = $1 AND counter_id = $2 AND state = 'completed'
			ORDER BY created_at DESC, operation_id DESC
			LIMIT $3
		`, tenantID, counterID, limit+1)
	} else {
		err = s.db.SelectContext(ctx, &rows, `
			SELECT operation_id, counter_id, kind, delta, value_before, value_after,
				actor_id, metadata, created_at, completed_at
			FROM counter_operations
			WHERE tenant_id = $1 AND counter_id = $2 AND state = 'completed'
			  AND (created_at, operation_id) < ($3, $4)
			ORDER BY created_at DESC, operation_id DESC
			LIMIT $5
		`, tenantID, counterID, cursor.CreatedAt, cursor.OperationID, limit+1)
	}
	if err != nil {
		return nil, fmt.Errorf("list counter operations: %w", err)
	}

	operations := make([]models.CounterOperation, 0, len(rows))
	for _, row := range rows {
		metadata := row.Metadata
		if len(metadata) == 0 {
			metadata = []byte(`{}`)
		}
		operations = append(operations, models.CounterOperation{
			OperationID: row.OperationID,
			CounterID:   row.CounterID,
			Kind:        row.Kind,
			Delta:       row.Delta,
			ValueBefore: row.ValueBefore,
			ValueAfter:  row.ValueAfter,
			ActorID:     row.ActorID,
			Metadata:    append([]byte(nil), metadata...),
			CreatedAt:   row.CreatedAt,
			CompletedAt: row.CompletedAt,
		})
	}
	return operations, nil
}

type reconciliationRow struct {
	TenantID          string `db:"tenant_id"`
	CounterID         string `db:"counter_id"`
	CurrentValue      int64  `db:"current_value"`
	TotalDelta        string `db:"total_delta"`
	OperationCount    int64  `db:"operation_count"`
	InitialValueCount int64  `db:"initial_value_count"`
}

// Reconcile checks counters in bounded batches and aggregates their completed
// operation history. It reports mismatches without changing application data.
func (s *CounterStore) Reconcile(ctx context.Context, batchSize int) (models.ReconciliationReport, error) {
	if batchSize < 1 {
		batchSize = 100
	}
	if batchSize > 1000 {
		batchSize = 1000
	}

	report := models.ReconciliationReport{StartedAt: time.Now().UTC()}
	var lastTenantID, lastCounterID string
	for {
		rows, err := s.reconciliationBatch(ctx, lastTenantID, lastCounterID, batchSize)
		if err != nil {
			return report, err
		}
		if len(rows) == 0 {
			break
		}

		for _, row := range rows {
			total, ok := new(big.Int).SetString(row.TotalDelta, 10)
			if !ok {
				return report, fmt.Errorf("parse total delta for counter %s/%s: %q", row.TenantID, row.CounterID, row.TotalDelta)
			}
			report.CountersChecked++
			report.OperationsScanned += row.OperationCount
			if total.Cmp(big.NewInt(row.CurrentValue)) != 0 {
				report.Mismatches++
			}
			if row.InitialValueCount != 1 {
				report.InitialValueViolations++
			}
		}

		last := rows[len(rows)-1]
		lastTenantID = last.TenantID
		lastCounterID = last.CounterID
	}
	report.CompletedAt = time.Now().UTC()
	return report, nil
}

func (s *CounterStore) reconciliationBatch(ctx context.Context, lastTenantID, lastCounterID string, batchSize int) ([]reconciliationRow, error) {
	var rows []reconciliationRow
	var err error
	if lastTenantID == "" {
		err = s.db.SelectContext(ctx, &rows, `
			SELECT c.tenant_id, c.id AS counter_id, c.value AS current_value,
				COALESCE(SUM(o.delta), 0)::text AS total_delta,
				COUNT(o.operation_id) AS operation_count,
				COUNT(o.operation_id) FILTER (WHERE o.kind = 'initial_value') AS initial_value_count
			FROM counters AS c
			LEFT JOIN counter_operations AS o
				ON o.tenant_id = c.tenant_id
				AND o.counter_id = c.id
				AND o.state = 'completed'
			GROUP BY c.tenant_id, c.id, c.value
			ORDER BY c.tenant_id, c.id
			LIMIT $1
		`, batchSize)
	} else {
		err = s.db.SelectContext(ctx, &rows, `
			SELECT c.tenant_id, c.id AS counter_id, c.value AS current_value,
				COALESCE(SUM(o.delta), 0)::text AS total_delta,
				COUNT(o.operation_id) AS operation_count,
				COUNT(o.operation_id) FILTER (WHERE o.kind = 'initial_value') AS initial_value_count
			FROM counters AS c
			LEFT JOIN counter_operations AS o
				ON o.tenant_id = c.tenant_id
				AND o.counter_id = c.id
				AND o.state = 'completed'
			WHERE (c.tenant_id, c.id) > ($1, $2)
			GROUP BY c.tenant_id, c.id, c.value
			ORDER BY c.tenant_id, c.id
			LIMIT $3
		`, lastTenantID, lastCounterID, batchSize)
	}
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reconcile counter batch: %w", err)
	}
	return rows, nil
}

var _ interface {
	ListOperations(context.Context, string, string, *models.OperationCursor, int) ([]models.CounterOperation, error)
	Reconcile(context.Context, int) (models.ReconciliationReport, error)
} = (*CounterStore)(nil)
