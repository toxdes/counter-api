package contract

import (
	"errors"
	"testing"
)

func TestValidateIdempotencyKeyContract(t *testing.T) {
	tests := []struct {
		name     string
		required bool
		key      string
		wantErr  error
	}{
		{
			name: "optional key may be omitted",
		},
		{
			name: "optional UUID key is accepted",
			key:  "01912345-6789-7000-8000-000000000001",
		},
		{
			name:     "required key cannot be omitted",
			required: true,
			wantErr:  ErrIdempotencyKeyRequired,
		},
		{
			name:     "required key must be a UUID",
			required: true,
			key:      "legacy-client-key",
			wantErr:  ErrInvalidIdempotencyKey,
		},
		{
			name:    "optional supplied key must be a UUID",
			key:     "not-a-uuid",
			wantErr: ErrInvalidIdempotencyKey,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateIdempotencyKey(tt.key, tt.required)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("ValidateIdempotencyKey() error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}
