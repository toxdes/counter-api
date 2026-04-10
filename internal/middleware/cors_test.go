package middleware

import (
	"testing"
)

func TestOriginMatches(t *testing.T) {
	tests := []struct {
		name        string
		origin      string
		allowed     string
		shouldMatch bool
	}{
		{
			name:        "exact match",
			origin:      "https://example.com",
			allowed:     "https://example.com",
			shouldMatch: true,
		},
		{
			name:        "wildcard subdomain match",
			origin:      "https://sub.example.com",
			allowed:     "https://*.example.com",
			shouldMatch: true,
		},
		{
			name:        "wildcard all match",
			origin:      "https://anything.com",
			allowed:     "*",
			shouldMatch: true,
		},
		{
			name:        "no match",
			origin:      "https://evil.com",
			allowed:     "https://example.com",
			shouldMatch: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matched := originMatches(tt.origin, tt.allowed)
			if matched != tt.shouldMatch {
				t.Errorf("originMatches() = %v, want %v", matched, tt.shouldMatch)
			}
		})
	}
}

func TestGetAllowedOrigin(t *testing.T) {
	allowed := "https://example.com,https://*.example.com"

	tests := []struct {
		name     string
		origin   string
		expected string
	}{
		{
			name:     "exact match",
			origin:   "https://example.com",
			expected: "https://example.com",
		},
		{
			name:     "wildcard match",
			origin:   "https://sub.example.com",
			expected: "https://*.example.com",
		},
		{
			name:     "no match",
			origin:   "https://evil.com",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getAllowedOrigin(tt.origin, allowed)
			if result != tt.expected {
				t.Errorf("getAllowedOrigin() = %s, want %s", result, tt.expected)
			}
		})
	}
}
