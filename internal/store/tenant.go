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

var (
	ErrTenantNotFound  = errors.New("tenant not found")
	ErrTenantConflict  = errors.New("tenant label already exists")
	ErrCounterNotFound = errors.New("counter not found")
	ErrCounterConflict = errors.New("counter label already exists")
)

// TenantStore contains PostgreSQL persistence for tenant operations.
type TenantStore struct {
	db *database.DB
}

func NewTenantStore(db *database.DB) *TenantStore {
	return &TenantStore{db: db}
}

func (s *TenantStore) CreateTenant(ctx context.Context, tenant *models.Tenant) error {
	_, err := s.db.ExecContext(ctx,
		"INSERT INTO tenants (id, label, created_at, updated_at) VALUES ($1, $2, $3, $4)",
		tenant.ID, tenant.Label, tenant.CreatedAt, tenant.UpdatedAt,
	)
	if err == nil {
		return nil
	}

	var pqErr *pq.Error
	if errors.As(err, &pqErr) && pqErr.Code == "23505" {
		return ErrTenantConflict
	}
	return fmt.Errorf("create tenant: %w", err)
}

func (s *TenantStore) GetTenant(ctx context.Context, id string) (*models.Tenant, error) {
	var tenant models.Tenant
	err := s.db.GetContext(ctx, &tenant,
		"SELECT id, label, created_at, updated_at FROM tenants WHERE id = $1",
		id,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrTenantNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get tenant: %w", err)
	}
	return &tenant, nil
}
