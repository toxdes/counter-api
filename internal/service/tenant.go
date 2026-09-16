package service

import (
	"context"
	"counter/internal/models"
	"counter/internal/store"
	"errors"
	"time"

	"github.com/google/uuid"
)

var (
	ErrNotFound = errors.New("not found")
	ErrConflict = errors.New("conflict")
)

// TenantRepository is the persistence boundary required by TenantService.
type TenantRepository interface {
	CreateTenant(context.Context, *models.Tenant) error
	GetTenant(context.Context, string) (*models.Tenant, error)
}

// TenantService contains tenant use cases independent of HTTP and SQL.
type TenantService interface {
	Create(context.Context, string) (*models.Tenant, error)
	Get(context.Context, string) (*models.Tenant, error)
}

type tenantService struct {
	repository TenantRepository
	now        func() time.Time
	newID      func() string
}

func NewTenantService(repository TenantRepository) TenantService {
	return &tenantService{
		repository: repository,
		now:        func() time.Time { return time.Now().UTC() },
		newID:      func() string { return uuid.NewString() },
	}
}

func NewTenantServiceWithDependencies(repository TenantRepository, now func() time.Time, newID func() string) TenantService {
	return &tenantService{repository: repository, now: now, newID: newID}
}

func (s *tenantService) Create(ctx context.Context, label string) (*models.Tenant, error) {
	now := s.now()
	tenant := &models.Tenant{ID: s.newID(), Label: label, CreatedAt: now, UpdatedAt: now}
	if err := s.repository.CreateTenant(ctx, tenant); err != nil {
		if errors.Is(err, store.ErrTenantConflict) {
			return nil, ErrConflict
		}
		return nil, err
	}
	return tenant, nil
}

func (s *tenantService) Get(ctx context.Context, id string) (*models.Tenant, error) {
	tenant, err := s.repository.GetTenant(ctx, id)
	if errors.Is(err, store.ErrTenantNotFound) {
		return nil, ErrNotFound
	}
	return tenant, err
}
