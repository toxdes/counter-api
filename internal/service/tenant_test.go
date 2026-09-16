package service

import (
	"context"
	"counter/internal/models"
	"counter/internal/store"
	"errors"
	"testing"
	"time"
)

type fakeTenantRepository struct {
	created   *models.Tenant
	createErr error
	gotten    *models.Tenant
	getErr    error
}

func (f *fakeTenantRepository) CreateTenant(_ context.Context, tenant *models.Tenant) error {
	f.created = tenant
	return f.createErr
}

func (f *fakeTenantRepository) GetTenant(_ context.Context, _ string) (*models.Tenant, error) {
	return f.gotten, f.getErr
}

func TestTenantServiceCreateInjectsIDAndClock(t *testing.T) {
	now := time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC)
	repository := &fakeTenantRepository{}
	service := NewTenantServiceWithDependencies(
		repository,
		func() time.Time { return now },
		func() string { return "tenant-id" },
	)

	created, err := service.Create(context.Background(), "acme")
	if err != nil {
		t.Fatalf("create tenant: %v", err)
	}
	if created != repository.created {
		t.Fatal("service did not return the tenant sent to the repository")
	}
	if created.ID != "tenant-id" || !created.CreatedAt.Equal(now) || !created.UpdatedAt.Equal(now) {
		t.Fatalf("created tenant = %#v", created)
	}
}

func TestTenantServiceTranslatesRepositoryErrors(t *testing.T) {
	tests := []struct {
		name          string
		repositoryErr error
		want          error
	}{
		{name: "conflict", repositoryErr: store.ErrTenantConflict, want: ErrConflict},
		{name: "not found", repositoryErr: store.ErrTenantNotFound, want: ErrNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repository := &fakeTenantRepository{createErr: tt.repositoryErr, getErr: tt.repositoryErr}
			service := NewTenantService(repository)

			if tt.name == "conflict" {
				if _, err := service.Create(context.Background(), "acme"); !errors.Is(err, tt.want) {
					t.Fatalf("create error = %v, want %v", err, tt.want)
				}
				return
			}
			if _, err := service.Get(context.Background(), "tenant-id"); !errors.Is(err, tt.want) {
				t.Fatalf("get error = %v, want %v", err, tt.want)
			}
		})
	}
}
