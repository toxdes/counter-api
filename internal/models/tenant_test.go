package models

import (
	"testing"
	"time"
)

func TestTenantValidate(t *testing.T) {
	tests := []struct {
		name    string
		tenant  *Tenant
		wantErr bool
	}{
		{
			name: "valid tenant",
			tenant: &Tenant{
				Label: "blog",
			},
			wantErr: false,
		},
		{
			name:    "empty label",
			tenant:  &Tenant{Label: ""},
			wantErr: true,
		},
		{
			name:    "nil tenant",
			tenant:  nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.tenant.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestTenantCreatedAt(t *testing.T) {
	tenant := &Tenant{
		ID:        "123e4567-e89b-12d3-a456-426614174000",
		Label:     "blog",
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}

	if tenant.ID == "" {
		t.Error("Expected ID to be set")
	}
	if tenant.Label != "blog" {
		t.Errorf("Expected label 'blog', got '%s'", tenant.Label)
	}
}
