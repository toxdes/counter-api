package store

import (
	"context"
	"counter/internal/database"
	"counter/internal/models"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/lib/pq"
)

var (
	ErrMetadataNotFound     = errors.New("metadata not found")
	ErrMetadataPrecondition = errors.New("metadata revision precondition failed")
	ErrInvalidMetadata      = errors.New("metadata cannot be represented as PostgreSQL JSONB")
)

type MetadataStore struct{ db *database.DB }

func NewMetadataStore(db *database.DB) *MetadataStore { return &MetadataStore{db: db} }

func (s *MetadataStore) CounterExists(ctx context.Context, tenantID, counterID string) (bool, error) {
	var exists bool
	err := database.ExecutorFor(ctx, s.db).GetContext(ctx, &exists, `SELECT EXISTS(SELECT 1 FROM counters WHERE tenant_id = $1 AND id = $2)`, tenantID, counterID)
	return exists, err
}

func (s *MetadataStore) Get(ctx context.Context, tenantID, counterID string) (*models.CounterMetadata, error) {
	var result models.CounterMetadata
	err := database.ExecutorFor(ctx, s.db).GetContext(ctx, &result, `SELECT tenant_id, counter_id, data, version, created_at, updated_at FROM counter_metadata WHERE tenant_id = $1 AND counter_id = $2`, tenantID, counterID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrMetadataNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get counter metadata: %w", err)
	}
	return &result, nil
}

func (s *MetadataStore) Put(ctx context.Context, tenantID, counterID string, data json.RawMessage, expected string, create bool) (*models.CounterMetadata, error) {
	tx, finish, err := database.BeginWrite(ctx, s.db)
	if err != nil {
		return nil, err
	}
	defer finish(false)
	exists, err := s.CounterExists(database.WithTransaction(ctx, tx), tenantID, counterID)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, ErrCounterNotFound
	}
	var result models.CounterMetadata
	if create {
		err = tx.GetContext(ctx, &result, `INSERT INTO counter_metadata (tenant_id, counter_id, data) VALUES ($1, $2, $3)
			ON CONFLICT (tenant_id, counter_id) DO NOTHING
			RETURNING tenant_id, counter_id, data, version, created_at, updated_at`, tenantID, counterID, []byte(data))
	} else {
		err = tx.GetContext(ctx, &result, `UPDATE counter_metadata SET data = $3, version = gen_random_uuid(), updated_at = clock_timestamp()
			WHERE tenant_id = $1 AND counter_id = $2 AND version = $4
			RETURNING tenant_id, counter_id, data, version, created_at, updated_at`, tenantID, counterID, []byte(data), expected)
	}
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrMetadataPrecondition
	}
	if err != nil {
		var postgresError *pq.Error
		if errors.As(err, &postgresError) {
			if len(postgresError.Code) >= 2 && string(postgresError.Code)[:2] == "22" {
				return nil, ErrInvalidMetadata
			}
			if postgresError.Code == "23503" {
				return nil, ErrCounterNotFound
			}
		}
		return nil, fmt.Errorf("replace counter metadata: %w", err)
	}
	if err := finish(true); err != nil {
		return nil, err
	}
	return &result, nil
}

func (s *MetadataStore) Delete(ctx context.Context, tenantID, counterID, expected string) error {
	tx, finish, err := database.BeginWrite(ctx, s.db)
	if err != nil {
		return err
	}
	defer finish(false)
	exists, err := s.CounterExists(database.WithTransaction(ctx, tx), tenantID, counterID)
	if err != nil {
		return err
	}
	if !exists {
		return ErrCounterNotFound
	}
	result, err := tx.ExecContext(ctx, `DELETE FROM counter_metadata WHERE tenant_id = $1 AND counter_id = $2 AND version = $3`, tenantID, counterID, expected)
	if err != nil {
		return fmt.Errorf("delete counter metadata: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return ErrMetadataPrecondition
	}
	return finish(true)
}
