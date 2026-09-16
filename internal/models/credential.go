package models

import "time"

// APICredential is the persisted, non-secret portion of an API credential.
// Verifier contains a one-way digest; the raw API key is never persisted.
type APICredential struct {
	ID          string     `json:"credential_id" db:"id"`
	TenantID    *string    `json:"tenant_id,omitempty" db:"tenant_id"`
	Verifier    []byte     `json:"-" db:"verifier"`
	Scopes      []string   `json:"scopes" db:"scopes"`
	IsAdmin     bool       `json:"is_admin" db:"is_admin"`
	CreatedAt   time.Time  `json:"created_at" db:"created_at"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty" db:"expires_at"`
	RevokedAt   *time.Time `json:"revoked_at,omitempty" db:"revoked_at"`
	CreatedBy   string     `json:"created_by,omitempty" db:"created_by"`
	RotatedFrom *string    `json:"rotated_from,omitempty" db:"rotated_from"`
}

// CredentialIssueResponse returns a newly generated API key exactly once.
type CredentialIssueResponse struct {
	CredentialID string     `json:"credential_id"`
	TenantID     string     `json:"tenant_id,omitempty"`
	APIKey       string     `json:"api_key"`
	Scopes       []string   `json:"scopes"`
	ExpiresAt    *time.Time `json:"expires_at,omitempty"`
}

// CredentialRequest contains the policy for a newly issued tenant key.
type CredentialRequest struct {
	Scopes    []string   `json:"scopes"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

// AuthAuditEvent records credential lifecycle and privileged actions without
// retaining raw secrets.
type AuthAuditEvent struct {
	ID           string    `db:"id"`
	CredentialID *string   `db:"credential_id"`
	TenantID     *string   `db:"tenant_id"`
	ActorID      string    `db:"actor_id"`
	Action       string    `db:"action"`
	Metadata     []byte    `db:"metadata"`
	CreatedAt    time.Time `db:"created_at"`
}
