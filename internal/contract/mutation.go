// Package contract contains API contract rules shared by V1 and V2
// transports. It deliberately does not contain database or HTTP code.
package contract

import (
	"errors"
	"strings"

	"github.com/google/uuid"
)

// Version identifies the public API contract a request is using.
type Version string

const (
	// V1 is the compatibility contract used by the existing unversioned API.
	V1 Version = "v1"
	// V2 is the stricter contract for new clients and new behavior.
	V2 Version = "v2"
)

var (
	// ErrIdempotencyKeyRequired is returned by V2 when a mutation has no key.
	ErrIdempotencyKeyRequired = errors.New("idempotency key is required")
	// ErrInvalidIdempotencyKey is returned when a supplied key is not a UUID.
	ErrInvalidIdempotencyKey = errors.New("idempotency key must be a UUID")
)

// ValidateIdempotencyKey enforces the version-specific key requirement.
// V1 preserves compatibility by allowing a missing key, while a supplied key
// is validated consistently in both versions. V2 requires a UUID key.
func ValidateIdempotencyKey(version Version, key string) error {
	key = strings.TrimSpace(key)
	if key == "" {
		if version == V1 {
			return nil
		}
		return ErrIdempotencyKeyRequired
	}

	if _, err := uuid.Parse(key); err != nil {
		return ErrInvalidIdempotencyKey
	}
	return nil
}
