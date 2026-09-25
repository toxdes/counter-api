package middleware

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"
)

type sharedLimiterStub struct {
	mu        sync.Mutex
	remaining map[string]int
	max       int
	err       error
}

func (s *sharedLimiterStub) AllowRequest(_ context.Context, key string, isGet bool, _ int, _ int64) (bool, int, error) {
	if s.err != nil {
		return false, 0, s.err
	}
	if isGet {
		key += ":get"
	} else {
		key += ":post"
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.remaining == nil {
		s.remaining = make(map[string]int)
	}
	remaining, exists := s.remaining[key]
	if !exists {
		remaining = s.max
	}
	if remaining <= 0 {
		return false, 1, nil
	}
	s.remaining[key] = remaining - 1
	return true, 0, nil
}

func TestSharedLimiterCoordinatesIndependentReplicas(t *testing.T) {
	shared := &sharedLimiterStub{max: 2}
	firstReplica := NewRateLimiter(10, 3, 60)
	secondReplica := NewRateLimiter(10, 3, 60)
	firstReplica.SetSharedBackend(shared)
	secondReplica.SetSharedBackend(shared)

	for i, replica := range []*RateLimiter{firstReplica, secondReplica} {
		allowed, _, fallback := replica.AllowRequestWithContext(context.Background(), "credential:tenant-key", false)
		if !allowed || fallback {
			t.Fatalf("replica %d request allowed=%v fallback=%v, want allowed without fallback", i, allowed, fallback)
		}
	}
	allowed, retryAfter, fallback := firstReplica.AllowRequestWithContext(context.Background(), "credential:tenant-key", false)
	if allowed || retryAfter <= 0 || fallback {
		t.Fatalf("shared quota result allowed=%v retry=%d fallback=%v, want shared rejection", allowed, retryAfter, fallback)
	}
}

func TestSharedLimiterFailureFallsBackToLocalWithoutRejectingAPI(t *testing.T) {
	limiter := NewRateLimiter(2, 1, 60)
	limiter.SetSharedBackend(&sharedLimiterStub{err: errors.New("redis unavailable")})

	for i := 0; i < 2; i++ {
		allowed, _, fallback := limiter.AllowRequestWithContext(context.Background(), "client", false)
		if !allowed || !fallback {
			t.Fatalf("local fallback request %d allowed=%v fallback=%v, want allowed via local limiter", i+1, allowed, fallback)
		}
	}
	allowed, retryAfter, fallback := limiter.AllowRequestWithContext(context.Background(), "client", false)
	if allowed || retryAfter <= 0 || !fallback {
		t.Fatalf("local quota result allowed=%v retry=%d fallback=%v, want local rejection", allowed, retryAfter, fallback)
	}
}

func TestRedisSharedLimitAcrossIndependentClients(t *testing.T) {
	redisURL := os.Getenv("RATE_LIMIT_REDIS_TEST_URL")
	if redisURL == "" {
		t.Skip("RATE_LIMIT_REDIS_TEST_URL is not configured")
	}

	firstBackend, err := NewRedisRateLimitBackend(redisURL)
	if err != nil {
		t.Fatalf("create first Redis backend: %v", err)
	}
	defer firstBackend.Close()
	secondBackend, err := NewRedisRateLimitBackend(redisURL)
	if err != nil {
		t.Fatalf("create second Redis backend: %v", err)
	}
	defer secondBackend.Close()

	firstReplica := NewRateLimiter(2, 1, 60)
	firstReplica.SetSharedBackend(firstBackend)
	secondReplica := NewRateLimiter(2, 1, 60)
	secondReplica.SetSharedBackend(secondBackend)
	key := fmt.Sprintf("redis-integration:%d", time.Now().UnixNano())

	for i, limiter := range []*RateLimiter{firstReplica, secondReplica} {
		allowed, _, fallback := limiter.AllowRequestWithContext(context.Background(), key, false)
		if !allowed || fallback {
			t.Fatalf("replica %d first request allowed=%v fallback=%v", i, allowed, fallback)
		}
	}
	allowed, retryAfter, fallback := firstReplica.AllowRequestWithContext(context.Background(), key, false)
	if allowed || retryAfter <= 0 || fallback {
		t.Fatalf("shared Redis result allowed=%v retry=%d fallback=%v", allowed, retryAfter, fallback)
	}
}
