package middleware

import (
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/valyala/fasthttp"
)

const (
	// MaxStoreEntries is the maximum number of IPs to track to prevent memory exhaustion
	MaxStoreEntries = 10000
)

// RateLimiter implements token bucket rate limiting
type RateLimiter struct {
	mu              sync.RWMutex
	store           map[string]*tokenBucket
	maxRequests     int
	maxGetRequests  int
	window          time.Duration
	cleanupInterval time.Duration
}

// tokenBucket represents a token bucket for a specific IP
type tokenBucket struct {
	postTokens     int
	getTokens      int
	maxPostTokens  int
	maxGetTokens   int
	postRefillRate int
	getRefillRate  int
	lastRefill     time.Time
	window         time.Duration
	mu             sync.Mutex
}

// NewRateLimiter creates a new rate limiter
func NewRateLimiter(maxRequests int, getMultiplier int, windowSeconds int) *RateLimiter {
	return &RateLimiter{
		store:           make(map[string]*tokenBucket),
		maxRequests:     maxRequests,
		maxGetRequests:  maxRequests * getMultiplier,
		window:          time.Duration(windowSeconds) * time.Second,
		cleanupInterval: time.Duration(windowSeconds) * time.Second,
	}
}

// AllowRequest checks if a request from the given IP should be allowed
func (rl *RateLimiter) AllowRequest(ip string, isGet bool) (bool, int) {
	rl.mu.Lock()

	// Prevent unbounded map growth - reject if too many entries
	if len(rl.store) >= MaxStoreEntries {
		rl.mu.Unlock()
		// Check if IP already exists (common case)
		rl.mu.RLock()
		_, exists := rl.store[ip]
		rl.mu.RUnlock()

		if !exists {
			// At capacity, reject request with short retry
			return false, 60
		}
	}

	bucket, exists := rl.store[ip]
	if !exists {
		bucket = &tokenBucket{
			postTokens:     rl.maxRequests,
			getTokens:      rl.maxGetRequests,
			maxPostTokens:  rl.maxRequests,
			maxGetTokens:   rl.maxGetRequests,
			postRefillRate: rl.maxRequests,
			getRefillRate:  rl.maxGetRequests,
			lastRefill:     time.Now(),
			window:         rl.window,
		}
		rl.store[ip] = bucket
	}
	rl.mu.Unlock()

	return bucket.allowRequest(isGet)
}

// Cleanup removes stale entries from the rate limiter
func (rl *RateLimiter) Cleanup(maxAge time.Duration) {
	rl.mu.Lock()

	// Collect IPs to delete first (don't delete while iterating)
	now := time.Now()
	var ipsToDelete []string

	for ip, bucket := range rl.store {
		bucket.mu.Lock()
		if now.Sub(bucket.lastRefill) > maxAge {
			ipsToDelete = append(ipsToDelete, ip)
		}
		bucket.mu.Unlock()
	}

	// Delete collected IPs
	for _, ip := range ipsToDelete {
		delete(rl.store, ip)
	}

	rl.mu.Unlock()
}

// GetMaxRequests returns the maximum requests for write operations
func (rl *RateLimiter) GetMaxRequests() int {
	return rl.maxRequests
}

// GetMaxGetRequests returns the maximum requests for GET operations
func (rl *RateLimiter) GetMaxGetRequests() int {
	return rl.maxGetRequests
}

// allowRequest checks if a request should be allowed (internal method)
func (tb *tokenBucket) allowRequest(isGet bool) (bool, int) {
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
		postRefillAmount := int(elapsed.Seconds()) * tb.postRefillRate / int(window.Seconds())
		getRefillAmount := int(elapsed.Seconds()) * tb.getRefillRate / int(window.Seconds())

		tb.postTokens += postRefillAmount
		tb.getTokens += getRefillAmount

		if tb.postTokens > tb.maxPostTokens {
			tb.postTokens = tb.maxPostTokens
		}
		if tb.getTokens > tb.maxGetTokens {
			tb.getTokens = tb.maxGetTokens
		}
		tb.lastRefill = now
	}

	// Check and consume appropriate token
	if isGet {
		if tb.getTokens > 0 {
			tb.getTokens--
			return true, 0
		}
	} else {
		if tb.postTokens > 0 {
			tb.postTokens--
			return true, 0
		}
	}

	// Calculate retry after
	retryAfter := int(window.Seconds() - elapsed.Seconds())
	if retryAfter < 0 {
		retryAfter = 0
	}
	return false, retryAfter
}

// AllowRequest checks if a request should be allowed (test helper method)
func (tb *tokenBucket) AllowRequest(isGet bool) bool {
	allowed, _ := tb.allowRequest(isGet)
	return allowed
}

// RateLimit returns a rate limiting middleware handler
func RateLimit(rl *RateLimiter) func(fasthttp.RequestHandler) fasthttp.RequestHandler {
	return func(next fasthttp.RequestHandler) fasthttp.RequestHandler {
		return func(ctx *fasthttp.RequestCtx) {
			// Check if this is a GET request (read operation)
			isGet := string(ctx.Method()) == "GET"

			// Get client IP
			ip := getClientIP(ctx)

			// Check if request should be allowed
			allowed, retryAfter := rl.AllowRequest(ip, isGet)

			// Set rate limit headers
			maxReq := rl.maxRequests
			if isGet {
				maxReq = rl.maxGetRequests
			}
			ctx.Response.Header.Set("X-RateLimit-Limit", strconv.Itoa(maxReq))

			if !allowed {
				ctx.Response.Header.Set("Retry-After", strconv.Itoa(retryAfter))
				ctx.Response.Header.SetContentType("application/json")
				ctx.SetStatusCode(fasthttp.StatusTooManyRequests)
				ctx.SetBodyString(`{"error":"RATE_LIMIT_EXCEEDED","message":"Too many requests. Please retry later."}`)
				return
			}

			next(ctx)
		}
	}
}

func getClientIP(ctx *fasthttp.RequestCtx) string {
	// IMPORTANT: Don't trust client-controlled headers for rate limiting
	// They can easily spoof IPs to bypass rate limiting
	//
	// Only trust X-Forwarded-For/X-Real-IP if from trusted proxy (localhost/private network)
	remoteIP := ctx.RemoteIP()

	// Check if request is from trusted proxy (localhost or private network)
	if isTrustedProxy(remoteIP) {
		// Try X-Real-IP first
		if ip := ctx.Request.Header.Peek("X-Real-IP"); len(ip) > 0 {
			parsedIP := net.ParseIP(string(ip))
			if parsedIP != nil {
				return parsedIP.String()
			}
		}

		// Try X-Forwarded-For (take first IP in chain)
		if ip := ctx.Request.Header.Peek("X-Forwarded-For"); len(ip) > 0 {
			ips := strings.Split(string(ip), ",")
			if len(ips) > 0 {
				parsedIP := net.ParseIP(strings.TrimSpace(ips[0]))
				if parsedIP != nil {
					return parsedIP.String()
				}
			}
		}
	}

	// Fall back to actual remote address
	return remoteIP.String()
}

// isTrustedProxy checks if an IP is from a trusted proxy (localhost/private network)
func isTrustedProxy(ip net.IP) bool {
	if ip.IsLoopback() {
		return true
	}

	if ip.IsPrivate() {
		return true
	}

	// IPv4 private ranges
	if ip4 := ip.To4(); ip4 != nil {
		// 10.0.0.0/8
		if ip4[0] == 10 {
			return true
		}
		// 172.16.0.0/12
		if ip4[0] == 172 && ip4[1] >= 16 && ip4[1] <= 31 {
			return true
		}
		// 192.168.0.0/16
		if ip4[0] == 192 && ip4[1] == 168 {
			return true
		}
	}

	return false
}
