package middleware

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"
)

const redisFallbackCooldown = time.Second

var errRedisRateLimitUnavailable = errors.New("redis rate limit backend temporarily unavailable")

// SharedRateLimitBackend stores request quota state shared by API replicas.
// maxRequests is the capacity and windowMicroseconds is the refill period.
type SharedRateLimitBackend interface {
	AllowRequest(context.Context, string, bool, int, int64) (bool, int, error)
}

// RedisRateLimitBackend stores token buckets atomically in Redis. It stores
// only hashed client or credential identifiers and expires idle buckets.
type RedisRateLimitBackend struct {
	client           *redis.Client
	unavailableUntil atomic.Int64
}

var redisTokenBucketScript = redis.NewScript(`
local time = redis.call('TIME')
local now = tonumber(time[1]) * 1000000 + tonumber(time[2])
local capacity = tonumber(ARGV[1])
local window_us = tonumber(ARGV[2])
local interval_us = window_us / capacity
if interval_us < 1 then interval_us = 1 end

local tokens = tonumber(redis.call('HGET', KEYS[1], 'tokens')) or capacity
local last = tonumber(redis.call('HGET', KEYS[1], 'last')) or now
if now > last then
  local refill = math.floor((now - last) / interval_us)
  if refill > 0 then
    tokens = math.min(capacity, tokens + refill)
    last = last + refill * interval_us
  end
end

local allowed = 0
local retry_us = 0
if tokens > 0 then
  tokens = tokens - 1
  allowed = 1
else
  retry_us = math.max(1, interval_us - (now - last))
end

redis.call('HSET', KEYS[1], 'tokens', tokens, 'last', last)
redis.call('PEXPIRE', KEYS[1], math.max(1000, math.ceil(window_us / 1000) * 2))
return {allowed, math.ceil(retry_us / 1000)}
`)

// NewRedisRateLimitBackend configures an optional Redis client without
// connecting during startup. Redis availability is checked per request with
// a short deadline; failures are handled by the local limiter.
func NewRedisRateLimitBackend(rawURL string) (*RedisRateLimitBackend, error) {
	options, err := redis.ParseURL(rawURL)
	if err != nil {
		return nil, fmt.Errorf("parse rate limit Redis URL: %w", err)
	}
	options.MaxRetries = -1
	options.DialTimeout = 50 * time.Millisecond
	options.ReadTimeout = 50 * time.Millisecond
	options.WriteTimeout = 50 * time.Millisecond
	options.PoolSize = 10
	return &RedisRateLimitBackend{client: redis.NewClient(options)}, nil
}

// AllowRequest executes the token-bucket transition atomically in Redis.
func (b *RedisRateLimitBackend) AllowRequest(ctx context.Context, key string, isGet bool, maxRequests int, windowMicroseconds int64) (bool, int, error) {
	if until := b.unavailableUntil.Load(); until > time.Now().UnixNano() {
		return false, 0, errRedisRateLimitUnavailable
	}
	if maxRequests < 1 || windowMicroseconds < 1 {
		return false, 0, errors.New("invalid shared rate limit parameters")
	}

	identifier := key + "\x00post"
	if isGet {
		identifier = key + "\x00get"
	}
	digest := sha256.Sum256([]byte(identifier))
	redisKey := "counter:rate-limit:" + hex.EncodeToString(digest[:])
	result, err := redisTokenBucketScript.Run(ctx, b.client, []string{redisKey}, maxRequests, windowMicroseconds).Int64Slice()
	if err != nil {
		b.unavailableUntil.Store(time.Now().Add(redisFallbackCooldown).UnixNano())
		return false, 0, fmt.Errorf("execute shared rate limit: %w", err)
	}
	if len(result) != 2 {
		b.unavailableUntil.Store(time.Now().Add(redisFallbackCooldown).UnixNano())
		return false, 0, fmt.Errorf("unexpected shared rate limit response length %d", len(result))
	}
	b.unavailableUntil.Store(0)
	return result[0] == 1, int(math.Max(0, float64(result[1]))), nil
}

// Close releases the optional Redis connection pool.
func (b *RedisRateLimitBackend) Close() error {
	return b.client.Close()
}
