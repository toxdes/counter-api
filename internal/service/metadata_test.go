package service

import (
	"errors"
	"strings"
	"testing"
)

func TestMetadataValidationBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name, data string
		want       error
	}{
		{"object", `{"a":[9007199254740993,true,null]}`, nil},
		{"array", `[]`, nil}, {"scalar", `"text"`, nil},
		{"number", `1e100`, nil}, {"boolean", `false`, nil}, {"null", `null`, nil},
		{"empty", ``, ErrInvalidMetadata}, {"trailing", `{} null`, ErrInvalidMetadata},
		{"depth limit", strings.Repeat("[", MaxMetadataDepth) + "0" + strings.Repeat("]", MaxMetadataDepth), nil},
		{"too deep", strings.Repeat("[", MaxMetadataDepth+1) + "0" + strings.Repeat("]", MaxMetadataDepth+1), ErrInvalidMetadata},
		{"size limit", `"` + strings.Repeat("x", MaxMetadataBytes-2) + `"`, nil},
		{"too large", `"` + strings.Repeat("x", MaxMetadataBytes-1) + `"`, ErrMetadataTooLarge},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := ValidateMetadata([]byte(tc.data)); !errors.Is(err, tc.want) {
				t.Fatalf("validation error=%v, want %v", err, tc.want)
			}
		})
	}
}
