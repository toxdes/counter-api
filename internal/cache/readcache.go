package cache

import (
	"container/list"
	"context"
	"counter/internal/models"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	redisReadCachePrefix   = "counter:read-cache:v1:"
	redisReadCacheCooldown = time.Second
)

// CounterReadCache is an optional response cache. PostgreSQL remains the
// authoritative store; failures from this interface must never fail API reads
// or writes.
type CounterReadCache interface {
	Get(context.Context, string, string) (*models.Counter, bool, error)
	Set(context.Context, *models.Counter) error
	Delete(context.Context, string, string) error
}

type readCacheEntry struct {
	key       string
	counter   models.Counter
	expiresAt time.Time
}

// MemoryCounterReadCache is a bounded, per-process LRU cache with a fixed TTL.
type MemoryCounterReadCache struct {
	mu      sync.Mutex
	maxSize int
	ttl     time.Duration
	items   map[string]*list.Element
	lru     *list.List
}

func NewMemoryCounterReadCache(maxSize int, ttl time.Duration) (*MemoryCounterReadCache, error) {
	if maxSize < 1 {
		return nil, errors.New("counter read cache max size must be positive")
	}
	if ttl <= 0 {
		return nil, errors.New("counter read cache TTL must be positive")
	}
	return &MemoryCounterReadCache{
		maxSize: maxSize,
		ttl:     ttl,
		items:   make(map[string]*list.Element),
		lru:     list.New(),
	}, nil
}

func (c *MemoryCounterReadCache) Get(_ context.Context, tenantID, counterID string) (*models.Counter, bool, error) {
	key := counterCacheKey(tenantID, counterID)
	c.mu.Lock()
	defer c.mu.Unlock()

	element, ok := c.items[key]
	if !ok {
		return nil, false, nil
	}
	entry := element.Value.(*readCacheEntry)
	if !time.Now().Before(entry.expiresAt) {
		c.remove(element)
		return nil, false, nil
	}
	c.lru.MoveToFront(element)
	counter := entry.counter
	return &counter, true, nil
}

func (c *MemoryCounterReadCache) Set(_ context.Context, counter *models.Counter) error {
	if counter == nil {
		return errors.New("cannot cache a nil counter")
	}
	key := counterCacheKey(counter.TenantID, counter.ID)
	c.mu.Lock()
	defer c.mu.Unlock()
	if element, ok := c.items[key]; ok {
		entry := element.Value.(*readCacheEntry)
		entry.counter = *counter
		entry.expiresAt = time.Now().Add(c.ttl)
		c.lru.MoveToFront(element)
		return nil
	}
	element := c.lru.PushFront(&readCacheEntry{key: key, counter: *counter, expiresAt: time.Now().Add(c.ttl)})
	c.items[key] = element
	for c.lru.Len() > c.maxSize {
		c.remove(c.lru.Back())
	}
	return nil
}

func (c *MemoryCounterReadCache) Delete(_ context.Context, tenantID, counterID string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if element, ok := c.items[counterCacheKey(tenantID, counterID)]; ok {
		c.remove(element)
	}
	return nil
}

func (c *MemoryCounterReadCache) remove(element *list.Element) {
	if element == nil {
		return
	}
	entry := element.Value.(*readCacheEntry)
	delete(c.items, entry.key)
	c.lru.Remove(element)
}

// RedisCounterReadCache shares cached counters across replicas. A sorted-set
// LRU and expiry index keep the configured capacity and TTL bounded globally.
type RedisCounterReadCache struct {
	client         *redis.Client
	maxSize        int
	ttl            time.Duration
	unavailableTil atomic.Int64
}

func NewRedisCounterReadCache(rawURL string, maxSize int, ttl time.Duration) (*RedisCounterReadCache, error) {
	if maxSize < 1 {
		return nil, errors.New("counter read cache max size must be positive")
	}
	if ttl <= 0 {
		return nil, errors.New("counter read cache TTL must be positive")
	}
	options, err := redis.ParseURL(rawURL)
	if err != nil {
		return nil, fmt.Errorf("parse counter read cache Redis URL: %w", err)
	}
	options.MaxRetries = -1
	options.DialTimeout = 50 * time.Millisecond
	options.ReadTimeout = 50 * time.Millisecond
	options.WriteTimeout = 50 * time.Millisecond
	options.PoolSize = 10
	return &RedisCounterReadCache{client: redis.NewClient(options), maxSize: maxSize, ttl: ttl}, nil
}

var redisCounterReadCacheGetScript = redis.NewScript(`
local value = redis.call('GET', KEYS[1])
if not value then
  redis.call('ZREM', KEYS[2], KEYS[1])
  redis.call('ZREM', KEYS[3], KEYS[1])
  return nil
end
local time = redis.call('TIME')
local now_us = tonumber(time[1]) * 1000000 + tonumber(time[2])
redis.call('ZADD', KEYS[2], now_us, KEYS[1])
return value
`)

var redisCounterReadCacheSetScript = redis.NewScript(`
local time = redis.call('TIME')
local now_us = tonumber(time[1]) * 1000000 + tonumber(time[2])
local ttl_ms = tonumber(ARGV[2])
local max_entries = tonumber(ARGV[3])
local expired = redis.call('ZRANGEBYSCORE', KEYS[3], '-inf', now_us)
for _, key in ipairs(expired) do
  redis.call('DEL', key)
  redis.call('ZREM', KEYS[2], key)
end
redis.call('SET', KEYS[1], ARGV[1], 'PX', ttl_ms)
redis.call('ZADD', KEYS[2], now_us, KEYS[1])
redis.call('ZADD', KEYS[3], now_us + ttl_ms * 1000, KEYS[1])
while redis.call('ZCARD', KEYS[2]) > max_entries do
  local oldest = redis.call('ZPOPMIN', KEYS[2], 1)
  if oldest[1] then
    redis.call('DEL', oldest[1])
    redis.call('ZREM', KEYS[3], oldest[1])
  end
end
return 1
`)

var redisCounterReadCacheDeleteScript = redis.NewScript(`
redis.call('DEL', KEYS[1])
redis.call('ZREM', KEYS[2], KEYS[1])
redis.call('ZREM', KEYS[3], KEYS[1])
return 1
`)

func (c *RedisCounterReadCache) Get(ctx context.Context, tenantID, counterID string) (*models.Counter, bool, error) {
	if err := c.available(); err != nil {
		return nil, false, err
	}
	key := c.redisKey(tenantID, counterID)
	rawValue, err := redisCounterReadCacheGetScript.Run(ctx, c.client, []string{key, redisReadCachePrefix + "lru", redisReadCachePrefix + "expiry"}).Result()
	if err == redis.Nil {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, c.markUnavailable(fmt.Errorf("read counter cache: %w", err))
	}
	if rawValue == nil {
		return nil, false, nil
	}
	var value []byte
	switch typedValue := rawValue.(type) {
	case string:
		value = []byte(typedValue)
	case []byte:
		value = typedValue
	default:
		return nil, false, c.markUnavailable(fmt.Errorf("unexpected counter cache value type %T", rawValue))
	}
	var counter models.Counter
	if err := json.Unmarshal(value, &counter); err != nil {
		_ = c.Delete(context.Background(), tenantID, counterID)
		return nil, false, nil
	}
	return &counter, true, nil
}

func (c *RedisCounterReadCache) Set(ctx context.Context, counter *models.Counter) error {
	if counter == nil {
		return errors.New("cannot cache a nil counter")
	}
	if err := c.available(); err != nil {
		return err
	}
	value, err := json.Marshal(counter)
	if err != nil {
		return fmt.Errorf("encode counter cache value: %w", err)
	}
	key := c.redisKey(counter.TenantID, counter.ID)
	ttlMilliseconds := c.ttl.Milliseconds()
	if ttlMilliseconds < 1 {
		ttlMilliseconds = 1
	}
	_, err = redisCounterReadCacheSetScript.Run(ctx, c.client, []string{key, redisReadCachePrefix + "lru", redisReadCachePrefix + "expiry"}, string(value), ttlMilliseconds, c.maxSize).Result()
	if err != nil {
		return c.markUnavailable(fmt.Errorf("write counter cache: %w", err))
	}
	c.unavailableTil.Store(0)
	return nil
}

func (c *RedisCounterReadCache) Delete(ctx context.Context, tenantID, counterID string) error {
	if err := c.available(); err != nil {
		return err
	}
	key := c.redisKey(tenantID, counterID)
	if _, err := redisCounterReadCacheDeleteScript.Run(ctx, c.client, []string{key, redisReadCachePrefix + "lru", redisReadCachePrefix + "expiry"}).Result(); err != nil {
		return c.markUnavailable(fmt.Errorf("invalidate counter cache: %w", err))
	}
	c.unavailableTil.Store(0)
	return nil
}

func (c *RedisCounterReadCache) Close() error {
	return c.client.Close()
}

func (c *RedisCounterReadCache) available() error {
	if until := c.unavailableTil.Load(); until > time.Now().UnixNano() {
		return errors.New("counter cache Redis is temporarily unavailable")
	}
	return nil
}

func (c *RedisCounterReadCache) markUnavailable(err error) error {
	c.unavailableTil.Store(time.Now().Add(redisReadCacheCooldown).UnixNano())
	return err
}

func (c *RedisCounterReadCache) redisKey(tenantID, counterID string) string {
	digest := sha256.Sum256([]byte(counterCacheKey(tenantID, counterID)))
	return redisReadCachePrefix + hex.EncodeToString(digest[:])
}

func counterCacheKey(tenantID, counterID string) string {
	return strings.ToLower(tenantID) + "\x00" + strings.ToLower(counterID)
}
