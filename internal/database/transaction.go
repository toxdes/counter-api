package database

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/jmoiron/sqlx"
)

type transactionKey struct{}

type accessKey struct{}

// Access is trusted server-side authorization context, never client-supplied
// tenant identity. PostgreSQL receives it only through transaction-local GUCs.
type Access struct {
	TenantID         string
	PrincipalID      string
	Admin            bool
	AuthenticationID string
}

func WithAccess(ctx context.Context, access Access) context.Context {
	return context.WithValue(ctx, accessKey{}, access)
}

func AccessFromContext(ctx context.Context) Access {
	access, _ := ctx.Value(accessKey{}).(Access)
	return access
}

// WithTransaction binds store writes to the transaction that also records
// their idempotency result. The owner alone commits or rolls it back.
func WithTransaction(ctx context.Context, tx *sqlx.Tx) context.Context {
	return context.WithValue(ctx, transactionKey{}, tx)
}

type Executor interface {
	ExecContext(context.Context, string, ...interface{}) (sql.Result, error)
	GetContext(context.Context, interface{}, string, ...interface{}) error
	SelectContext(context.Context, interface{}, string, ...interface{}) error
}

func ExecutorFor(ctx context.Context, db *DB) Executor {
	if tx, ok := ctx.Value(transactionKey{}).(*sqlx.Tx); ok {
		return tx
	}
	return scopedExecutor{db}
}

// BeginWrite joins an outer transaction when present. finish(false) is safe
// to defer; finish(true) commits only transactions owned by this call.
func BeginWrite(ctx context.Context, db *DB) (*sqlx.Tx, func(bool) error, error) {
	if tx, ok := ctx.Value(transactionKey{}).(*sqlx.Tx); ok {
		return tx, func(bool) error { return nil }, nil
	}
	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, nil, err
	}
	access := AccessFromContext(ctx)
	_, err = tx.ExecContext(ctx, `SELECT
		set_config('counter.tenant_id', $1, true),
		set_config('counter.principal_id', $2, true),
		set_config('counter.is_admin', $3, true),
		set_config('counter.auth_credential_id', $4, true)`, access.TenantID, access.PrincipalID, fmt.Sprint(access.Admin), access.AuthenticationID)
	if err != nil {
		_ = tx.Rollback()
		return nil, nil, fmt.Errorf("set transaction access: %w", err)
	}
	return tx, func(commit bool) error {
		if commit {
			return tx.Commit()
		}
		return tx.Rollback()
	}, nil
}

type scopedExecutor struct{ db *DB }

func (e scopedExecutor) GetContext(ctx context.Context, dest interface{}, query string, args ...interface{}) error {
	tx, finish, err := BeginWrite(ctx, e.db)
	if err != nil {
		return err
	}
	defer finish(false)
	if err := tx.GetContext(ctx, dest, query, args...); err != nil {
		return err
	}
	return finish(true)
}

func (e scopedExecutor) SelectContext(ctx context.Context, dest interface{}, query string, args ...interface{}) error {
	tx, finish, err := BeginWrite(ctx, e.db)
	if err != nil {
		return err
	}
	defer finish(false)
	if err := tx.SelectContext(ctx, dest, query, args...); err != nil {
		return err
	}
	return finish(true)
}

func (e scopedExecutor) ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
	tx, finish, err := BeginWrite(ctx, e.db)
	if err != nil {
		return nil, err
	}
	defer finish(false)
	result, err := tx.ExecContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	if err := finish(true); err != nil {
		return nil, err
	}
	return result, nil
}
