package service

import (
	"context"
	"counter/internal/models"
	"counter/internal/security"
	"counter/internal/store"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/google/uuid"
)

var (
	ErrCredentialNotFound = errors.New("credential not found")
	ErrInvalidCredential  = errors.New("invalid credential")
)

// CredentialRepository is the persistence boundary for credential lifecycle
// operations and their audit records.
type CredentialRepository interface {
	GetActiveCredential(context.Context, string) (*models.APICredential, error)
	CreateCredential(context.Context, *models.APICredential) error
	RevokeCredential(context.Context, string, time.Time) error
	CreateAuthAuditEvent(context.Context, *models.AuthAuditEvent) error
	CreateCredentialAndAudit(context.Context, *models.APICredential, *models.AuthAuditEvent) error
	RevokeCredentialAndAudit(context.Context, string, time.Time, *models.AuthAuditEvent) error
}

// CredentialService manages tenant API credentials without exposing stored
// verifiers or raw secrets after issuance.
type CredentialService interface {
	Create(context.Context, string, []string, *time.Time, string) (*models.CredentialIssueResponse, error)
	CreateAdmin(context.Context, []string, *time.Time, string) (*models.CredentialIssueResponse, error)
	Rotate(context.Context, string, string, string) (*models.CredentialIssueResponse, error)
	Revoke(context.Context, string, string, string) error
}

type credentialService struct {
	repository CredentialRepository
	now        func() time.Time
	newID      func() string
}

func NewCredentialService(repository CredentialRepository) CredentialService {
	return &credentialService{
		repository: repository,
		now:        func() time.Time { return time.Now().UTC() },
		newID:      func() string { return uuid.NewString() },
	}
}

func (s *credentialService) Create(ctx context.Context, tenantID string, scopes []string, expiresAt *time.Time, actorID string) (*models.CredentialIssueResponse, error) {
	return s.create(ctx, stringPointer(tenantID), false, scopes, expiresAt, actorID)
}

func (s *credentialService) CreateAdmin(ctx context.Context, scopes []string, expiresAt *time.Time, actorID string) (*models.CredentialIssueResponse, error) {
	return s.create(ctx, nil, true, scopes, expiresAt, actorID)
}

func (s *credentialService) create(ctx context.Context, tenantID *string, isAdmin bool, scopes []string, expiresAt *time.Time, actorID string) (*models.CredentialIssueResponse, error) {
	scopes, err := normalizeCredentialScopes(scopes)
	if err != nil {
		return nil, err
	}
	if expiresAt != nil && !expiresAt.After(s.now()) {
		return nil, fmt.Errorf("credential expiry must be in the future")
	}

	credentialID := s.newID()
	apiKey, verifier, err := security.GenerateAPIKey(credentialID)
	if err != nil {
		return nil, err
	}
	credential := &models.APICredential{
		ID:        credentialID,
		TenantID:  tenantID,
		Verifier:  verifier,
		Scopes:    scopes,
		IsAdmin:   isAdmin,
		CreatedAt: s.now(),
		ExpiresAt: expiresAt,
		CreatedBy: actorID,
	}
	if err := s.repository.CreateCredentialAndAudit(ctx, credential, s.auditEvent(credential, actorID, "created")); err != nil {
		if errors.Is(err, store.ErrTenantNotFound) {
			return nil, ErrTenantNotFound
		}
		return nil, err
	}
	return issueResponse(credential, apiKey), nil
}

func (s *credentialService) Rotate(ctx context.Context, tenantID, credentialID, actorID string) (*models.CredentialIssueResponse, error) {
	old, err := s.repository.GetActiveCredential(ctx, credentialID)
	if errors.Is(err, store.ErrCredentialNotFound) || err == nil && !credentialBelongsToTenant(old, tenantID) {
		return nil, ErrCredentialNotFound
	}
	if err != nil {
		return nil, err
	}

	newID := s.newID()
	apiKey, verifier, err := security.GenerateAPIKey(newID)
	if err != nil {
		return nil, err
	}
	credential := &models.APICredential{
		ID:          newID,
		TenantID:    old.TenantID,
		Verifier:    verifier,
		Scopes:      append([]string(nil), old.Scopes...),
		CreatedAt:   s.now(),
		ExpiresAt:   old.ExpiresAt,
		CreatedBy:   actorID,
		RotatedFrom: &old.ID,
	}
	if err := s.repository.CreateCredentialAndAudit(ctx, credential, s.auditEvent(credential, actorID, "rotated")); err != nil {
		return nil, err
	}
	return issueResponse(credential, apiKey), nil
}

func (s *credentialService) Revoke(ctx context.Context, tenantID, credentialID, actorID string) error {
	credential, err := s.repository.GetActiveCredential(ctx, credentialID)
	if errors.Is(err, store.ErrCredentialNotFound) || err == nil && !credentialBelongsToTenant(credential, tenantID) {
		return ErrCredentialNotFound
	}
	if err != nil {
		return err
	}
	revokedAt := s.now()
	if err := s.repository.RevokeCredentialAndAudit(ctx, credentialID, revokedAt, s.auditEvent(credential, actorID, "revoked")); err != nil {
		if errors.Is(err, store.ErrCredentialNotFound) {
			return ErrCredentialNotFound
		}
		return err
	}
	return nil
}

func (s *credentialService) auditEvent(credential *models.APICredential, actorID, action string) *models.AuthAuditEvent {
	return &models.AuthAuditEvent{
		ID:           s.newID(),
		CredentialID: stringPointer(credential.ID),
		TenantID:     credential.TenantID,
		ActorID:      actorID,
		Action:       action,
		Metadata:     []byte(`{}`),
		CreatedAt:    s.now(),
	}
}

func issueResponse(credential *models.APICredential, apiKey string) *models.CredentialIssueResponse {
	tenantID := ""
	if credential.TenantID != nil {
		tenantID = *credential.TenantID
	}
	return &models.CredentialIssueResponse{
		CredentialID: credential.ID,
		TenantID:     tenantID,
		APIKey:       apiKey,
		Scopes:       append([]string(nil), credential.Scopes...),
		ExpiresAt:    credential.ExpiresAt,
	}
}

func normalizeCredentialScopes(scopes []string) ([]string, error) {
	if len(scopes) == 0 {
		scopes = []string{"counter:read", "counter:increment", "counter:history"}
	}
	allowed := map[string]bool{
		"tenant:read":       true,
		"counter:read":      true,
		"counter:create":    true,
		"counter:increment": true,
		"counter:adjust":    true,
		"counter:history":   true,
	}
	seen := make(map[string]bool, len(scopes))
	result := make([]string, 0, len(scopes))
	for _, scope := range scopes {
		if !allowed[scope] {
			return nil, fmt.Errorf("unsupported credential scope %q", scope)
		}
		if !seen[scope] {
			seen[scope] = true
			result = append(result, scope)
		}
	}
	sort.Strings(result)
	return result, nil
}

func credentialBelongsToTenant(credential *models.APICredential, tenantID string) bool {
	return credential != nil && credential.TenantID != nil && *credential.TenantID == tenantID
}

func stringPointer(value string) *string {
	return &value
}
