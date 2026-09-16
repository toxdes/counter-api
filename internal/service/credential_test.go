package service

import (
	"context"
	"counter/internal/models"
	"counter/internal/security"
	"errors"
	"testing"
	"time"
)

type fakeCredentialRepository struct {
	credentials map[string]*models.APICredential
	audits      []models.AuthAuditEvent
}

func (f *fakeCredentialRepository) GetActiveCredential(_ context.Context, id string) (*models.APICredential, error) {
	credential := f.credentials[id]
	if credential == nil || credential.RevokedAt != nil {
		return nil, nil
	}
	return credential, nil
}

func (f *fakeCredentialRepository) CreateCredential(_ context.Context, credential *models.APICredential) error {
	if f.credentials == nil {
		f.credentials = make(map[string]*models.APICredential)
	}
	f.credentials[credential.ID] = credential
	return nil
}

func (f *fakeCredentialRepository) RevokeCredential(_ context.Context, id string, revokedAt time.Time) error {
	credential := f.credentials[id]
	if credential == nil || credential.RevokedAt != nil {
		return errors.New("credential not found")
	}
	credential.RevokedAt = &revokedAt
	return nil
}

func (f *fakeCredentialRepository) CreateAuthAuditEvent(_ context.Context, event *models.AuthAuditEvent) error {
	f.audits = append(f.audits, *event)
	return nil
}

func (f *fakeCredentialRepository) CreateCredentialAndAudit(ctx context.Context, credential *models.APICredential, event *models.AuthAuditEvent) error {
	if err := f.CreateCredential(ctx, credential); err != nil {
		return err
	}
	return f.CreateAuthAuditEvent(ctx, event)
}

func (f *fakeCredentialRepository) RevokeCredentialAndAudit(ctx context.Context, id string, revokedAt time.Time, event *models.AuthAuditEvent) error {
	if err := f.RevokeCredential(ctx, id, revokedAt); err != nil {
		return err
	}
	return f.CreateAuthAuditEvent(ctx, event)
}

func TestCredentialServiceCreatesRotatesAndRevokesKeys(t *testing.T) {
	repository := &fakeCredentialRepository{}
	credentialService := NewCredentialService(repository)

	created, err := credentialService.Create(context.Background(), "tenant-a", nil, nil, "admin")
	if err != nil {
		t.Fatalf("Create() failed: %v", err)
	}
	if created.APIKey == "" || len(repository.audits) != 1 {
		t.Fatalf("created = %#v, audits = %d", created, len(repository.audits))
	}
	credentialID, secret, err := security.ParseAPIKey(created.APIKey)
	if err != nil || credentialID != created.CredentialID || secret == "" {
		t.Fatalf("generated key = %q, parse error = %v", created.APIKey, err)
	}

	rotated, err := credentialService.Rotate(context.Background(), "tenant-a", created.CredentialID, "admin")
	if err != nil {
		t.Fatalf("Rotate() failed: %v", err)
	}
	if rotated.APIKey == created.APIKey || len(repository.audits) != 2 {
		t.Fatalf("rotation did not create an overlapping new key: %#v", rotated)
	}
	if _, err := repository.GetActiveCredential(context.Background(), created.CredentialID); err != nil {
		t.Fatalf("old credential should remain active during overlap: %v", err)
	}

	if err := credentialService.Revoke(context.Background(), "tenant-a", created.CredentialID, "admin"); err != nil {
		t.Fatalf("Revoke() failed: %v", err)
	}
	if credential, _ := repository.GetActiveCredential(context.Background(), created.CredentialID); credential != nil {
		t.Fatal("revoked credential should no longer be active")
	}
	if len(repository.audits) != 3 || repository.audits[2].Action != "revoked" {
		t.Fatalf("audit events = %#v", repository.audits)
	}
}

func TestCredentialServiceCreatesAdministratorCredential(t *testing.T) {
	repository := &fakeCredentialRepository{}
	credentialService := NewCredentialService(repository)

	created, err := credentialService.CreateAdmin(context.Background(), nil, nil, "legacy-env-admin")
	if err != nil {
		t.Fatalf("CreateAdmin() failed: %v", err)
	}
	credential := repository.credentials[created.CredentialID]
	if credential == nil || !credential.IsAdmin || credential.TenantID != nil {
		t.Fatalf("stored administrator credential = %#v", credential)
	}
	if created.TenantID != "" || len(repository.audits) != 1 || repository.audits[0].Action != "created" {
		t.Fatalf("created response/audit = %#v/%#v", created, repository.audits)
	}
}
