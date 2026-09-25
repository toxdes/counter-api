package store

import (
	"context"
	"counter/internal/database"
	"counter/internal/models"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/lib/pq"
)

var (
	ErrCredentialNotFound = errors.New("credential not found")
	ErrCredentialRevoked  = errors.New("credential revoked")
)

// CredentialStore contains PostgreSQL persistence for API credentials.
type CredentialStore struct {
	db *database.DB
}

type credentialRow struct {
	ID          string         `db:"id"`
	TenantID    *string        `db:"tenant_id"`
	Verifier    []byte         `db:"verifier"`
	Scopes      pq.StringArray `db:"scopes"`
	IsAdmin     bool           `db:"is_admin"`
	CreatedAt   time.Time      `db:"created_at"`
	ExpiresAt   *time.Time     `db:"expires_at"`
	RevokedAt   *time.Time     `db:"revoked_at"`
	CreatedBy   string         `db:"created_by"`
	RotatedFrom *string        `db:"rotated_from"`
}

func NewCredentialStore(db *database.DB) *CredentialStore {
	return &CredentialStore{db: db}
}

func (s *CredentialStore) GetActiveCredential(ctx context.Context, credentialID string) (*models.APICredential, error) {
	var row credentialRow
	err := s.db.GetContext(ctx, &row, `
		SELECT id, tenant_id, verifier, scopes, is_admin, created_at,
			expires_at, revoked_at, created_by, rotated_from
		FROM api_credentials
		WHERE id = $1
		  AND revoked_at IS NULL
		  AND (expires_at IS NULL OR expires_at > NOW())
	`, credentialID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get active credential: %w", err)
	}
	return &models.APICredential{
		ID:          row.ID,
		TenantID:    row.TenantID,
		Verifier:    row.Verifier,
		Scopes:      []string(row.Scopes),
		IsAdmin:     row.IsAdmin,
		CreatedAt:   row.CreatedAt,
		ExpiresAt:   row.ExpiresAt,
		RevokedAt:   row.RevokedAt,
		CreatedBy:   row.CreatedBy,
		RotatedFrom: row.RotatedFrom,
	}, nil
}

func (s *CredentialStore) CreateCredential(ctx context.Context, credential *models.APICredential) error {
	_, err := insertCredential(ctx, s.db, credential)
	if err != nil {
		return mapCredentialWriteError(err)
	}
	return nil
}

func (s *CredentialStore) CreateCredentialAndAudit(ctx context.Context, credential *models.APICredential, event *models.AuthAuditEvent) error {
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin credential creation: %w", err)
	}
	defer tx.Rollback()
	if _, err := insertCredential(ctx, tx, credential); err != nil {
		return mapCredentialWriteError(err)
	}
	if _, err := insertAuthAuditEvent(ctx, tx, event); err != nil {
		return fmt.Errorf("create credential audit event: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit credential creation: %w", err)
	}
	return nil
}

type credentialExecer interface {
	ExecContext(context.Context, string, ...interface{}) (sql.Result, error)
}

func insertCredential(ctx context.Context, execer credentialExecer, credential *models.APICredential) (sql.Result, error) {
	return execer.ExecContext(ctx, `
		INSERT INTO api_credentials (
			id, tenant_id, verifier, scopes, is_admin, created_at,
			expires_at, created_by, rotated_from
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`, credential.ID, credential.TenantID, credential.Verifier, pq.Array(credential.Scopes), credential.IsAdmin,
		credential.CreatedAt, credential.ExpiresAt, credential.CreatedBy, credential.RotatedFrom)
}

func mapCredentialWriteError(err error) error {
	if err != nil {
		var pqErr *pq.Error
		if errors.As(err, &pqErr) && pqErr.Code == "23503" {
			return ErrTenantNotFound
		}
		return fmt.Errorf("create credential: %w", err)
	}
	return nil
}

func (s *CredentialStore) RevokeCredential(ctx context.Context, credentialID string, revokedAt time.Time) error {
	result, err := s.db.ExecContext(ctx, `
		UPDATE api_credentials
		SET revoked_at = $2
		WHERE id = $1 AND revoked_at IS NULL
	`, credentialID, revokedAt)
	if err != nil {
		return fmt.Errorf("revoke credential: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check revoked credential: %w", err)
	}
	if count == 0 {
		return ErrCredentialNotFound
	}
	return nil
}

func (s *CredentialStore) RevokeCredentialAndAudit(ctx context.Context, credentialID string, revokedAt time.Time, event *models.AuthAuditEvent) error {
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin credential revocation: %w", err)
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `
		UPDATE api_credentials
		SET revoked_at = $2
		WHERE id = $1 AND revoked_at IS NULL
	`, credentialID, revokedAt)
	if err != nil {
		return fmt.Errorf("revoke credential: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check revoked credential: %w", err)
	}
	if count == 0 {
		return ErrCredentialNotFound
	}
	if _, err := insertAuthAuditEvent(ctx, tx, event); err != nil {
		return fmt.Errorf("create revocation audit event: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit credential revocation: %w", err)
	}
	return nil
}

func (s *CredentialStore) CreateAuthAuditEvent(ctx context.Context, event *models.AuthAuditEvent) error {
	_, err := insertAuthAuditEvent(ctx, s.db, event)
	if err != nil {
		return fmt.Errorf("create auth audit event: %w", err)
	}
	return nil
}

func insertAuthAuditEvent(ctx context.Context, execer credentialExecer, event *models.AuthAuditEvent) (sql.Result, error) {
	return execer.ExecContext(ctx, `
		INSERT INTO auth_audit_events (
			id, credential_id, tenant_id, actor_id, action, metadata, created_at
		) VALUES ($1, $2, $3, $4, $5, COALESCE($6, '{}'::jsonb), $7)
	`, event.ID, event.CredentialID, event.TenantID, event.ActorID, event.Action, event.Metadata, event.CreatedAt)
}

var _ interface {
	GetActiveCredential(context.Context, string) (*models.APICredential, error)
	CreateCredential(context.Context, *models.APICredential) error
	RevokeCredential(context.Context, string, time.Time) error
	CreateAuthAuditEvent(context.Context, *models.AuthAuditEvent) error
	CreateCredentialAndAudit(context.Context, *models.APICredential, *models.AuthAuditEvent) error
	RevokeCredentialAndAudit(context.Context, string, time.Time, *models.AuthAuditEvent) error
} = (*CredentialStore)(nil)
