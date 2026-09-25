package contract

import (
	"errors"
	"testing"
)

func TestValidateIdempotencyKeyContract(t *testing.T) {
	tests := []struct {
		name    string
		version Version
		key     string
		wantErr error
	}{
		{
			name:    "v1 may omit key",
			version: V1,
		},
		{
			name:    "v1 accepts UUID key",
			version: V1,
			key:     "01912345-6789-7000-8000-000000000001",
		},
		{
			name:    "v2 requires key",
			version: V2,
			wantErr: ErrIdempotencyKeyRequired,
		},
		{
			name:    "v2 requires UUID key",
			version: V2,
			key:     "legacy-client-key",
			wantErr: ErrInvalidIdempotencyKey,
		},
		{
			name:    "both versions reject invalid supplied key",
			version: V1,
			key:     "not-a-uuid",
			wantErr: ErrInvalidIdempotencyKey,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateIdempotencyKey(tt.version, tt.key)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("ValidateIdempotencyKey() error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}
