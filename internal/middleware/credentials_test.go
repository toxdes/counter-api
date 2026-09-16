package middleware

import (
	"context"
	"counter/internal/models"
	"testing"
	"time"
)

type fakeCredentialRepository struct {
	credential *models.APICredential
	requested  string
}

func (f *fakeCredentialRepository) GetActiveCredential(_ context.Context, credentialID string) (*models.APICredential, error) {
	f.requested = credentialID
	return f.credential, nil
}

func TestAPIKeyAuthenticatorReturnsScopedPrincipal(t *testing.T) {
	const credentialID = "123e4567-e89b-12d3-a456-426614174002"
	const tenantID = "123e4567-e89b-12d3-a456-426614174003"
	const secret = "test-secret-that-is-long-enough"
	tenant := tenantID

	repository := &fakeCredentialRepository{credential: &models.APICredential{
		ID:        credentialID,
		TenantID:  &tenant,
		Verifier:  hashAPIKey(secret),
		Scopes:    []string{ScopeCounterRead, ScopeCounterIncrement},
		CreatedAt: time.Now().UTC(),
	}}
	authenticator := NewAPIKeyAuthenticator("", repository)

	principal, err := authenticator.Authenticate(context.Background(), formatAPIKey(credentialID, secret))
	if err != nil {
		t.Fatalf("Authenticate() failed: %v", err)
	}
	if repository.requested != credentialID {
		t.Fatalf("repository credential ID = %q, want %q", repository.requested, credentialID)
	}
	if principal.CredentialID != credentialID || principal.TenantID != tenantID || principal.IsAdmin {
		t.Fatalf("principal = %#v", principal)
	}
	if !principal.HasScope(ScopeCounterIncrement) || principal.HasScope(ScopeCounterAdjust) {
		t.Fatalf("principal scopes = %#v", principal.Scopes)
	}
}

func TestPrincipalAuthorizationIsTenantAndScopeBound(t *testing.T) {
	principal := Principal{TenantID: "tenant-a", Scopes: []string{ScopeCounterRead}}
	if !PrincipalAuthorized(principal, "tenant-a", ScopeCounterRead) {
		t.Fatal("matching tenant and scope should be authorized")
	}
	if PrincipalAuthorized(principal, "tenant-b", ScopeCounterRead) {
		t.Fatal("credential must not authorize another tenant")
	}
	if PrincipalAuthorized(principal, "tenant-a", ScopeCounterAdjust) {
		t.Fatal("credential must not authorize an ungranted scope")
	}
	admin := Principal{IsAdmin: true}
	if !PrincipalAuthorized(admin, "any-tenant", ScopeCounterAdjust) {
		t.Fatal("admin should authorize privileged scopes")
	}
}
