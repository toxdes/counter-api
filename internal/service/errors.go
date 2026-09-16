package service

import "errors"

var (
	ErrNotFound            = errors.New("not found")
	ErrTenantNotFound      = errors.New("tenant not found")
	ErrCounterNotFound     = errors.New("counter not found")
	ErrConflict            = errors.New("conflict")
	ErrDeltaExceedsMaximum = errors.New("delta exceeds maximum")
)
