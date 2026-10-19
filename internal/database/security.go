package database

import (
	"context"
	"fmt"
)

// VerifyRuntimeRole prevents an apparently protected deployment from silently
// bypassing RLS. Migrations must use a separate owner connection.
func VerifyRuntimeRole(ctx context.Context, db *DB) error {
	var unsafe bool
	if err := db.GetContext(ctx, &unsafe, `SELECT rolsuper OR rolbypassrls FROM pg_roles WHERE rolname = current_user`); err != nil {
		return fmt.Errorf("verify runtime role: %w", err)
	}
	if unsafe {
		return fmt.Errorf("runtime database role must not be superuser or BYPASSRLS; use a restricted role and a separate migration owner")
	}
	if err := db.GetContext(ctx, &unsafe, `SELECT EXISTS (
		SELECT 1 FROM pg_class c
		WHERE c.oid IN ('tenants'::regclass, 'counters'::regclass,
		'counter_operations'::regclass, 'counter_metadata'::regclass,
		'api_credentials'::regclass, 'auth_audit_events'::regclass, 'idempotency_requests'::regclass)
		AND pg_has_role(current_user, c.relowner, 'MEMBER'))`); err != nil {
		return fmt.Errorf("verify runtime ownership: %w", err)
	}
	if unsafe {
		return fmt.Errorf("runtime role must not own protected tables or be a member of their owner role")
	}
	return nil
}
