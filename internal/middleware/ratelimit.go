package middleware

import (
	"strconv"
	"sync"
	"time"

	"github.com/valyala/fasthttp"
)

// RateLimiter implements token bucket rate limiting
type RateLimiter struct {
	mu              sync.RWMutex
	store           map[string]*tokenBucket
	maxRequests     int
	window          time.Duration
	cleanupInterval time.Duration
}

// tokenBucket represents a token bucket for a specific IP
type tokenBucket struct {
	tokens     int
	maxTokens  int
	refillRate int
	lastRefill time.Time
	window     time.Duration
	mu         sync.Mutex
}

// NewRateLimiter creates a new rate limiter
func NewRateLimiter(maxRequests int, windowSeconds int) *RateLimiter {
	return &RateLimiter{
		store:           make(map[string]*tokenBucket),
		maxRequests:     maxRequests,
		window:          time.Duration(windowSeconds) * time.Second,
		cleanupInterval: time.Duration(windowSeconds) * time.Second,
	}
}

// AllowRequest checks if a request from the given IP should be allowed
func (rl *RateLimiter) AllowRequest(ip string) (bool, int) {
	rl.mu.Lock()
	bucket, exists := rl.store[ip]
	if !exists {
		bucket = &tokenBucket{
			tokens:     rl.maxRequests,
			maxTokens:  rl.maxRequests,
			refillRate: rl.maxRequests,
			lastRefill: time.Now(),
			window:     rl.window,
		}
		rl.store[ip] = bucket
	}
	rl.mu.Unlock()

	return bucket.allowRequest()
}

// Cleanup removes stale entries from the rate limiter
func (rl *RateLimiter) Cleanup(maxAge time.Duration) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	for ip, bucket := range rl.store {
		bucket.mu.Lock()
		if now.Sub(bucket.lastRefill) > maxAge {
			delete(rl.store, ip)
		}
		bucket.mu.Unlock()
	}
}

// allowRequest checks if a request should be allowed (internal method)
func (tb *tokenBucket) allowRequest() (bool, int) {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(tb.lastRefill)

	// Refill tokens based on elapsed time
	window := tb.window
	if window == 0 {
		// Default to 1 second window if not set (for test compatibility)
		window = time.Second
	}

	if elapsed > 0 {
		refillAmount := int(elapsed.Seconds()) * tb.refillRate / int(window.Seconds())
		tb.tokens += refillAmount
		if tb.tokens > tb.maxTokens {
			tb.tokens = tb.maxTokens
		}
		tb.lastRefill = now
	}

	if tb.tokens > 0 {
		tb.tokens--
		return true, 0
	}

	// Calculate retry after
	retryAfter := int(window.Seconds() - elapsed.Seconds())
	if retryAfter < 0 {
		retryAfter = 0
	}
	return false, retryAfter
}

// AllowRequest checks if a request should be allowed (test helper method)
func (tb *tokenBucket) AllowRequest() bool {
	allowed, _ := tb.allowRequest()
	return allowed
}

// RateLimit returns a rate limiting middleware handler
func RateLimit(rl *RateLimiter) func(fasthttp.RequestHandler) fasthttp.RequestHandler {
	return func(next fasthttp.RequestHandler) fasthttp.RequestHandler {
		return func(ctx *fasthttp.RequestCtx) {
			// Only rate limit POST requests
			if string(ctx.Method()) != "POST" {
				next(ctx)
				return
			}

			// Get client IP
			ip := getClientIP(ctx)

			// Check if request should be allowed
			allowed, retryAfter := rl.AllowRequest(ip)

			// Set rate limit headers
			ctx.Response.Header.Set("X-RateLimit-Limit", strconv.Itoa(rl.maxRequests))

			if !allowed {
				ctx.Response.Header.Set("Retry-After", strconv.Itoa(retryAfter))
				ctx.SetStatusCode(fasthttp.StatusTooManyRequests)
				return
			}

			next(ctx)
		}
	}
}

func getClientIP(ctx *fasthttp.RequestCtx) string {
	// Check X-Real-IP header
	if ip := ctx.Request.Header.Peek("X-Real-IP"); len(ip) > 0 {
		return string(ip)
	}

	// Check X-Forwarded-For header
	if ip := ctx.Request.Header.Peek("X-Forwarded-For"); len(ip) > 0 {
		return string(ip)
	}

	// Fall back to remote address
	return ctx.RemoteIP().String()
}
