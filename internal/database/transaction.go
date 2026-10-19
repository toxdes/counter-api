package database

import (
	"context"
	"database/sql"

	"github.com/jmoiron/sqlx"
)

type transactionKey struct{}

// WithTransaction binds store writes to the transaction that also records
// their idempotency result. The owner alone commits or rolls it back.
func WithTransaction(ctx context.Context, tx *sqlx.Tx) context.Context {
	return context.WithValue(ctx, transactionKey{}, tx)
}

type Executor interface {
	ExecContext(context.Context, string, ...interface{}) (sql.Result, error)
	GetContext(context.Context, interface{}, string, ...interface{}) error
}

func ExecutorFor(ctx context.Context, db *DB) Executor {
	if tx, ok := ctx.Value(transactionKey{}).(*sqlx.Tx); ok {
		return tx
	}
	return db
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
	return tx, func(commit bool) error {
		if commit {
			return tx.Commit()
		}
		return tx.Rollback()
	}, nil
}
