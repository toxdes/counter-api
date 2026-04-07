package middleware

import (
	"testing"
	"time"
)

func TestTokenBucketAllowRequest(t *testing.T) {
	tb := &tokenBucket{
		tokens:      10,
		maxTokens:   10,
		refillRate:  10,
		lastRefill:  time.Now(),
	}

	// First 10 requests should be allowed
	for i := 0; i < 10; i++ {
		if !tb.AllowRequest() {
			t.Errorf("Request %d should be allowed", i+1)
		}
	}

	// 11th request should be denied
	if tb.AllowRequest() {
		t.Error("Request 11 should be denied")
	}
}

func TestTokenBucketRefill(t *testing.T) {
	tb := &tokenBucket{
		tokens:      0,
		maxTokens:   10,
		refillRate:  10,
		lastRefill:  time.Now().Add(-time.Second),
	}

	// Should refill tokens
	if !tb.AllowRequest() {
		t.Error("Request should be allowed after refill")
	}
}

func TestRateLimiter(t *testing.T) {
	rl := NewRateLimiter(10, 60)

	// Allow 10 requests
	for i := 0; i < 10; i++ {
		allowed, retryAfter := rl.AllowRequest("192.168.1.1")
		if !allowed {
			t.Errorf("Request %d should be allowed", i+1)
		}
		if retryAfter != 0 {
			t.Errorf("RetryAfter should be 0 for allowed requests, got %d", retryAfter)
		}
	}

	// 11th request should be denied
	allowed, retryAfter := rl.AllowRequest("192.168.1.1")
	if allowed {
		t.Error("Request 11 should be denied")
	}
	if retryAfter <= 0 {
		t.Error("RetryAfter should be positive for denied requests")
	}
}

func TestRateLimiterCleanup(t *testing.T) {
	rl := NewRateLimiter(10, 60)

	// Add an entry
	rl.AllowRequest("192.168.1.1")

	// Wait and cleanup
	time.Sleep(100 * time.Millisecond)
	rl.Cleanup(50 * time.Millisecond)

	// The entry should be cleaned up
	rl.mu.RLock()
	_, exists := rl.store["192.168.1.1"]
	rl.mu.RUnlock()

	if exists {
		t.Error("Old entry should be cleaned up")
	}
}
