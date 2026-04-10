package middleware

import (
	"testing"
	"time"
)

func TestTokenBucketAllowRequest(t *testing.T) {
	tb := &tokenBucket{
		postTokens:     10,
		getTokens:      30,
		maxPostTokens:  10,
		maxGetTokens:   30,
		postRefillRate: 10,
		getRefillRate:  30,
		lastRefill:     time.Now(),
	}

	// First 10 POST requests should be allowed
	for i := 0; i < 10; i++ {
		if !tb.AllowRequest(false) {
			t.Errorf("POST request %d should be allowed", i+1)
		}
	}

	// 11th POST request should be denied
	if tb.AllowRequest(false) {
		t.Error("POST request 11 should be denied")
	}

	// But GET requests should still work (separate bucket)
	for i := 0; i < 30; i++ {
		if !tb.AllowRequest(true) {
			t.Errorf("GET request %d should be allowed", i+1)
		}
	}

	// 31st GET request should be denied
	if tb.AllowRequest(true) {
		t.Error("GET request 31 should be denied")
	}
}

func TestTokenBucketRefill(t *testing.T) {
	tb := &tokenBucket{
		postTokens:     0,
		getTokens:      0,
		maxPostTokens:  10,
		maxGetTokens:   30,
		postRefillRate: 10,
		getRefillRate:  30,
		lastRefill:     time.Now().Add(-time.Second),
	}

	// Should refill POST tokens
	if !tb.AllowRequest(false) {
		t.Error("POST request should be allowed after refill")
	}

	// Should refill GET tokens
	if !tb.AllowRequest(true) {
		t.Error("GET request should be allowed after refill")
	}
}

func TestRateLimiter(t *testing.T) {
	rl := NewRateLimiter(10, 1, 60)

	// Allow 10 requests
	for i := 0; i < 10; i++ {
		allowed, retryAfter := rl.AllowRequest("192.168.1.1", false)
		if !allowed {
			t.Errorf("Request %d should be allowed", i+1)
		}
		if retryAfter != 0 {
			t.Errorf("RetryAfter should be 0 for allowed requests, got %d", retryAfter)
		}
	}

	// 11th request should be denied
	allowed, retryAfter := rl.AllowRequest("192.168.1.1", false)
	if allowed {
		t.Error("Request 11 should be denied")
	}
	if retryAfter <= 0 {
		t.Error("RetryAfter should be positive for denied requests")
	}
}

func TestRateLimiterCleanup(t *testing.T) {
	rl := NewRateLimiter(10, 1, 60)

	// Add an entry
	rl.AllowRequest("192.168.1.1", false)

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
