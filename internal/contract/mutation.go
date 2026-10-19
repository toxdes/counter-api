// Package contract contains API contract rules independent of HTTP and storage.
package contract

import (
	"errors"
	"strings"

	"github.com/google/uuid"
)

var (
	// ErrIdempotencyKeyRequired is returned when a required key is missing.
	ErrIdempotencyKeyRequired = errors.New("idempotency key is required")
	// ErrInvalidIdempotencyKey is returned when a supplied key is not a UUID.
	ErrInvalidIdempotencyKey = errors.New("idempotency key must be a UUID")
)

// ValidateIdempotencyKey validates a UUID key. Increment/set require a key;
// creation and credential lifecycle operations accept an optional key.
func ValidateIdempotencyKey(key string, required bool) error {
	key = strings.TrimSpace(key)
	if key == "" {
		if !required {
			return nil
		}
		return ErrIdempotencyKeyRequired
	}

	if _, err := uuid.Parse(key); err != nil {
		return ErrInvalidIdempotencyKey
	}
	return nil
}
