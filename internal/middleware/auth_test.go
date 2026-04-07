package middleware

import (
	"testing"

	"github.com/valyala/fasthttp"
)

func TestAPIKeyAuth(t *testing.T) {
	apiKey := "test-api-key"
	handler := APIKeyAuth(apiKey)(func(ctx *fasthttp.RequestCtx) {
		ctx.SetStatusCode(fasthttp.StatusOK)
	})

	tests := []struct {
		name       string
		apiKey     string
		expectedStatus int
	}{
		{
			name:       "valid API key",
			apiKey:     "test-api-key",
			expectedStatus: fasthttp.StatusOK,
		},
		{
			name:       "missing API key",
			apiKey:     "",
			expectedStatus: fasthttp.StatusUnauthorized,
		},
		{
			name:       "invalid API key",
			apiKey:     "wrong-key",
			expectedStatus: fasthttp.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := &fasthttp.RequestCtx{}
			if tt.apiKey != "" {
				ctx.Request.Header.Set("X-API-Key", tt.apiKey)
			}

			handler(ctx)

			if ctx.Response.StatusCode() != tt.expectedStatus {
				t.Errorf("Expected status %d, got %d", tt.expectedStatus, ctx.Response.StatusCode())
			}
		})
	}
}
