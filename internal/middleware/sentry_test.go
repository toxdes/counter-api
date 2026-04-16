package middleware

import (
	"context"
	"testing"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/valyala/fasthttp"
)

func TestExtractContext(t *testing.T) {
	tests := []struct {
		name              string
		path              string
		expectedTenantID  string
		expectedCounterID string
	}{
		{
			name:              "full path with tenant and counter",
			path:              "/tenants/abc123/counters/def456",
			expectedTenantID:  "abc123",
			expectedCounterID: "def456",
		},
		{
			name:              "path with only tenant",
			path:              "/tenants/abc123",
			expectedTenantID:  "abc123",
			expectedCounterID: "",
		},
		{
			name:              "path without tenant or counter",
			path:              "/health",
			expectedTenantID:  "",
			expectedCounterID: "",
		},
		{
			name:              "empty path",
			path:              "",
			expectedTenantID:  "",
			expectedCounterID: "",
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

// TestNewSentryHandler_PanicRecovery tests that panics are recovered and sent to Sentry
func TestNewSentryHandler_PanicRecovery(t *testing.T) {
	// Initialize Sentry (in memory transport for testing)
	events := make(chan *sentry.Event, 100)
	transport := &mockTransport{events: events}

	err := sentry.Init(sentry.ClientOptions{
		Dsn:              "https://key@sentry.io/123",
		Transport:        transport,
		AttachStacktrace: true,
	})
	require.NoError(t, err)
	defer sentry.Flush(2 * time.Second)

	// Track if panic handler was called
	handlerCalled := false

	// Create a handler that panics
	panicHandler := func(ctx *fasthttp.RequestCtx) {
		handlerCalled = true
		panic("test panic")
	}

	// Wrap with Sentry middleware
	sentryHandler := NewSentryHandler(panicHandler)

	// Create test context
	ctx := &fasthttp.RequestCtx{}
	ctx.Request.SetRequestURI("http://example.com/tenants/tenant1/counters/counter1")

	// This should not panic (it's recovered by middleware)
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("Panic was not recovered by middleware: %v", r)
			}
		}()
		sentryHandler(ctx)
	}()

	// Verify handler was called
	assert.True(t, handlerCalled, "Handler should have been called")

	// Give time for event to be captured
	time.Sleep(100 * time.Millisecond)

	// Check if event was captured
	t.Logf("Events channel size: %d", len(events))
	select {
	case event := <-events:
		t.Logf("Event captured - Message: %s, Exceptions: %d", event.Message, len(event.Exception))
		// Event should be captured, either as exception or message
		if len(event.Exception) > 0 {
			assert.Equal(t, "test panic", event.Exception[0].Value)
		} else {
			// If no exception, at least check that some event was captured
			assert.NotEmpty(t, event.Message, "Expected event message or exception")
		}
	default:
		t.Error("Expected Sentry event to be captured, but none was")
	}
}

// TestNewSentryHandler_ErrorCapture tests that 5xx and 429 errors are captured
func TestNewSentryHandler_ErrorCapture(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		shouldSend bool
	}{
		{"500 Internal Server Error", 500, true},
		{"502 Bad Gateway", 502, true},
		{"503 Service Unavailable", 503, true},
		{"429 Too Many Requests", 429, true},
		{"404 Not Found", 404, false},
		{"400 Bad Request", 400, false},
		{"200 OK", 200, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Initialize Sentry
			events := make(chan *sentry.Event, 100)
			err := sentry.Init(sentry.ClientOptions{
				Dsn:       "https://key@sentry.io/123",
				Transport: &mockTransport{events: events},
			})
			require.NoError(t, err)
			defer sentry.Flush(100 * time.Millisecond)

			// Create a handler that returns the test status code
			testHandler := func(ctx *fasthttp.RequestCtx) {
				ctx.SetStatusCode(tt.statusCode)
			}

			// Wrap with Sentry middleware
			sentryHandler := NewSentryHandler(testHandler)

			// Create test context
			ctx := &fasthttp.RequestCtx{}
			ctx.Request.SetRequestURI("http://example.com/tenants/tenant1/counters/counter1")

			// Call handler
			sentryHandler(ctx)

			// Give time for event to be captured
			time.Sleep(50 * time.Millisecond)

			// Check if event was captured
			if tt.shouldSend {
				select {
				case event := <-events:
					assert.NotEmpty(t, event.Message, "Expected error message to be captured")
				default:
					t.Error("Expected Sentry event to be captured, but none was")
				}
			} else {
				select {
				case <-events:
					t.Error("Expected no Sentry event to be captured, but one was")
				default:
					// No event, as expected
				}
			}
		})
	}
}

// TestNewSentryHandler_ContextExtraction tests that context is properly extracted
func TestNewSentryHandler_ContextExtraction(t *testing.T) {
	// Initialize Sentry
	events := make(chan *sentry.Event, 100)
	err := sentry.Init(sentry.ClientOptions{
		Dsn:       "https://key@sentry.io/123",
		Transport: &mockTransport{events: events},
	})
	require.NoError(t, err)
	defer sentry.Flush(100 * time.Millisecond)

	// Create a handler that returns 500 to trigger event
	testHandler := func(ctx *fasthttp.RequestCtx) {
		ctx.SetStatusCode(500)
	}

	// Wrap with Sentry middleware
	sentryHandler := NewSentryHandler(testHandler)

	// Create test context
	ctx := &fasthttp.RequestCtx{}
	ctx.Request.SetRequestURI("http://example.com/tenants/test_tenant/counters/test_counter")

	// Call handler
	sentryHandler(ctx)

	// Give time for event to be captured
	time.Sleep(50 * time.Millisecond)

	// Check captured event has correct context
	select {
	case event := <-events:
		// Check tags
		assert.Equal(t, "test_tenant", event.Tags["tenant_id"])
		assert.Equal(t, "test_counter", event.Tags["counter_id"])
		assert.Equal(t, "GET", event.Tags["method"])
	default:
		t.Error("Expected Sentry event to be captured, but none was")
	}
}

// TestNewSentryHandler_NoSentryInit tests behavior when Sentry is not initialized
func TestNewSentryHandler_NoSentryInit(t *testing.T) {
	// Create a normal handler
	testHandler := func(ctx *fasthttp.RequestCtx) {
		ctx.SetStatusCode(200)
		ctx.WriteString("OK")
	}

	// Wrap with Sentry middleware (without initializing Sentry)
	sentryHandler := NewSentryHandler(testHandler)

	// Create test context
	ctx := &fasthttp.RequestCtx{}
	ctx.Request.SetRequestURI("http://example.com/test")

	// Call handler - should work even without Sentry initialized
	sentryHandler(ctx)

	// Should still get response even though Sentry is not initialized
	assert.Equal(t, 200, ctx.Response.StatusCode())
	assert.Equal(t, "OK", string(ctx.Response.Body()))
}

// mockTransport is a mock transport that sends events to a channel instead of Sentry
type mockTransport struct {
	events chan *sentry.Event
}

func (t *mockTransport) Configure(options sentry.ClientOptions) {
}

func (t *mockTransport) SendEvent(event *sentry.Event) {
	if t.events != nil {
		t.events <- event
	}
}

func (t *mockTransport) Flush(timeout time.Duration) bool {
	return true
}

func (t *mockTransport) FlushWithContext(ctx context.Context) bool {
	return true
}

func (t *mockTransport) Close() {
}
