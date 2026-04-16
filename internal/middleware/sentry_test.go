package middleware

import (
	"testing"

	"github.com/valyala/fasthttp"
	"github.com/stretchr/testify/assert"
)

func TestExtractContext(t *testing.T) {
	tests := []struct {
		name                string
		path                string
		expectedTenantID    string
		expectedCounterID   string
	}{
		{
			name:                "full path with tenant and counter",
			path:                "/tenants/abc123/counters/def456",
			expectedTenantID:    "abc123",
			expectedCounterID:   "def456",
		},
		{
			name:                "path with only tenant",
			path:                "/tenants/abc123",
			expectedTenantID:    "abc123",
			expectedCounterID:   "",
		},
		{
			name:                "path without tenant or counter",
			path:                "/health",
			expectedTenantID:    "",
			expectedCounterID:   "",
		},
		{
			name:                "empty path",
			path:                "",
			expectedTenantID:    "",
			expectedCounterID:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := &fasthttp.RequestCtx{}
			ctx.Request.SetRequestURI(tt.path)

			tenantID, counterID := extractSentryContext(ctx)

			assert.Equal(t, tt.expectedTenantID, tenantID)
			assert.Equal(t, tt.expectedCounterID, counterID)
		})
	}
}

func TestShouldCaptureError(t *testing.T) {
	tests := []struct {
		name           string
		statusCode     int
		expectedResult bool
	}{
		{
			name:           "500 internal server error",
			statusCode:     500,
			expectedResult: true,
		},
		{
			name:           "503 service unavailable",
			statusCode:     503,
			expectedResult: true,
		},
		{
			name:           "429 rate limit exceeded",
			statusCode:     429,
			expectedResult: true,
		},
		{
			name:           "404 not found",
			statusCode:     404,
			expectedResult: false,
		},
		{
			name:           "400 bad request",
			statusCode:     400,
			expectedResult: false,
		},
		{
			name:           "200 ok",
			statusCode:     200,
			expectedResult: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := shouldCaptureError(tt.statusCode)
			assert.Equal(t, tt.expectedResult, result)
		})
	}
}
