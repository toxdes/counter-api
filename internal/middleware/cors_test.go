package middleware

import (
	"testing"
)

func TestIsOriginAllowed(t *testing.T) {
	tests := []struct {
		name          string
		origin        string
		allowedOrigins string
		shouldMatch   bool
	}{
		{
			name:          "exact match",
			origin:        "https://example.com",
			allowedOrigins: "https://example.com,https://other.com",
			shouldMatch:   true,
		},
		{
			name:          "wildcard all match",
			origin:        "https://anything.com",
			allowedOrigins: "*",
			shouldMatch:   true,
		},
		{
			name:          "no match",
			origin:        "https://evil.com",
			allowedOrigins: "https://example.com",
			shouldMatch:   false,
		},
		{
			name:          "multiple origins match second",
			origin:        "https://other.com",
			allowedOrigins: "https://example.com,https://other.com",
			shouldMatch:   true,
		},
		{
			name:          "whitespace in list",
			origin:        "https://example.com",
			allowedOrigins: "https://example.com , https://other.com",
			shouldMatch:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matched := isOriginAllowed(tt.origin, tt.allowedOrigins)
			if matched != tt.shouldMatch {
				t.Errorf("isOriginAllowed() = %v, want %v", matched, tt.shouldMatch)
			}
		})
	}
}
