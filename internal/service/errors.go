package service

import "errors"

var (
	ErrNotFound             = errors.New("not found")
	ErrTenantNotFound       = errors.New("tenant not found")
	ErrCounterNotFound      = errors.New("counter not found")
	ErrConflict             = errors.New("conflict")
	ErrDeltaExceedsMaximum  = errors.New("delta exceeds maximum")
	ErrIdempotencyKeyReused = errors.New("idempotency key reused")
	ErrOperationInProgress  = errors.New("operation is already in progress")
	ErrCounterOverflow      = errors.New("counter overflow")
)
