package models

import (
	"errors"
	"time"
)

// Tenant represents a tenant in the system
type Tenant struct {
	ID        string    `json:"tenant_id" db:"id"`
	Label     string    `json:"label" db:"label"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
	UpdatedAt time.Time `json:"updated_at" db:"updated_at"`
}

// Validate validates the tenant data
func (t *Tenant) Validate() error {
	if t == nil {
		return errors.New("tenant is nil")
	}
	if t.Label == "" {
		return errors.New("label is required")
	}
	return nil
}

// CreateTenantRequest represents a request to create a tenant
type CreateTenantRequest struct {
	Label string `json:"label"`
}

// Validate validates the create tenant request
func (r *CreateTenantRequest) Validate() error {
	if r.Label == "" {
		return errors.New("label is required")
	}
	return nil
}
