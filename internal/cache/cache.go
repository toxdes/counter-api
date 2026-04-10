package cache

import (
	"container/list"
	"context"
	"counter/internal/models"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"
)

// LRUCache implements a thread-safe LRU (Least Recently Used) cache
type LRUCache struct {
	maxSize int
	items   map[string]*list.Element
	lru     *list.List
	mu      sync.RWMutex

	// Metrics
	hits   atomic.Int64
	misses atomic.Int64
}

// lruItem represents an item in the cache with its metadata
type lruItem struct {
	key        string
	value      *models.Counter
	expiration time.Time
}

// NewLRUCache creates a new LRU cache with the specified maximum size
func NewLRUCache(maxSize int) *LRUCache {
	return &LRUCache{
		maxSize: maxSize,
		items:   make(map[string]*list.Element),
		lru:     list.New(),
	}
}

// Get retrieves a value from the cache. Returns (value, true) if found,
// (nil, false) if not found or expired.
func (c *LRUCache) Get(key string) (*models.Counter, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	element, exists := c.items[key]
	if !exists {
		c.misses.Add(1)
		return nil, false
	}

	item := element.Value.(*lruItem)

	// Check expiration
	if !item.expiration.IsZero() && time.Now().After(item.expiration) {
		// Item expired, remove it
		c.removeElement(element)
		c.misses.Add(1)
		return nil, false
	}

	// Move to front (most recently used)
	c.lru.MoveToFront(element)
	c.hits.Add(1)

	return item.value, true
}

// Put adds a value to the cache. If the key already exists, it updates the value.
// If the cache is full, it evicts the least recently used item.
func (c *LRUCache) Put(key string, value *models.Counter, ttl ...time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	var expiration time.Time
	if len(ttl) > 0 && ttl[0] > 0 {
		expiration = time.Now().Add(ttl[0])
	}

	// Check if key already exists
	if element, exists := c.items[key]; exists {
		// Update existing item
		item := element.Value.(*lruItem)
		item.value = value
		item.expiration = expiration
		c.lru.MoveToFront(element)
		return
	}

	// Add new item
	item := &lruItem{
		key:        key,
		value:      value,
		expiration: expiration,
	}

	element := c.lru.PushFront(item)
	c.items[key] = element

	// Check if we need to evict
	if c.lru.Len() > c.maxSize {
		c.evictOldest()
	}
}

// Delete removes a key from the cache
func (c *LRUCache) Delete(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if element, exists := c.items[key]; exists {
		c.removeElement(element)
	}
}

// Clear removes all items from the cache
func (c *LRUCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.items = make(map[string]*list.Element)
	c.lru.Init()
	c.hits.Store(0)
	c.misses.Store(0)
}

// Size returns the current number of items in the cache
func (c *LRUCache) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.lru.Len()
}

// Stats returns cache statistics
func (c *LRUCache) Stats() CacheStats {
	hits := c.hits.Load()
	misses := c.misses.Load()
	total := hits + misses

	hitRate := 0.0
	if total > 0 {
		hitRate = float64(hits) / float64(total)
	}

	return CacheStats{
		Hits:    hits,
		Misses:  misses,
		HitRate: hitRate,
		Size:    c.Size(),
	}
}

// evictOldest removes the least recently used item from the cache
func (c *LRUCache) evictOldest() {
	element := c.lru.Back()
	if element != nil {
		c.removeElement(element)
	}
}

// removeElement removes an element from the cache
func (c *LRUCache) removeElement(element *list.Element) {
	item := element.Value.(*lruItem)
	delete(c.items, item.key)
	c.lru.Remove(element)
}

// CacheStats represents cache performance statistics
type CacheStats struct {
	Hits    int64
	Misses  int64
	HitRate float64
	Size    int
}

// WriteQueue manages asynchronous write operations to the database
type WriteQueue struct {
	cache         *LRUCache
	db            any // Use database.DB in actual implementation
	workers       int
	queueSize     int
	writeCh       chan writeRequest
	shutdownCh    chan struct{}
	wg            sync.WaitGroup
	writeFunc     WriteFunc
	droppedWrites atomic.Int64
}

// writeRequest represents a write operation request
type writeRequest struct {
	key      string
	delta    int64
	setValue *int64 // For set operations
}

// WriteFunc is the function type for writing to the database
type WriteFunc func(counterID string, delta int64) error

// NewWriteQueue creates a new write queue for async database operations
func NewWriteQueue(cache *LRUCache, db any, workers, queueSize int) *WriteQueue {
	return &WriteQueue{
		cache:      cache,
		db:         db,
		workers:    workers,
		queueSize:  queueSize,
		writeCh:    make(chan writeRequest, queueSize),
		shutdownCh: make(chan struct{}),
	}
}

// Start begins processing write requests with worker goroutines
func (wq *WriteQueue) Start(ctx context.Context) {
	for i := 0; i < wq.workers; i++ {
		wq.wg.Add(1)
		go wq.worker(ctx, i)
	}
}

// worker processes write requests from the queue
func (wq *WriteQueue) worker(ctx context.Context, id int) {
	defer wq.wg.Done()

	log.Printf("Cache write worker %d started", id)

	for {
		select {
		case <-ctx.Done():
			log.Printf("Cache write worker %d stopping: context cancelled", id)
			return
		case <-wq.shutdownCh:
			log.Printf("Cache write worker %d stopping: shutdown requested", id)
			return
		case req := <-wq.writeCh:
			if err := wq.processWrite(req); err != nil {
				log.Printf("Cache write worker %d: failed to process write for key %s: %v",
					id, req.key, err)
			}
		}
	}
}

// processWrite handles a single write request
func (wq *WriteQueue) processWrite(req writeRequest) error {
	// Extract counter ID from key (format: tenant_id:counter_id)
	counterID := extractCounterID(req.key)

	if wq.writeFunc != nil {
		return wq.writeFunc(counterID, req.delta)
	}

	// Default implementation (will be replaced with actual DB write)
	// This is a placeholder - in real usage, writeFunc will be set
	return fmt.Errorf("write function not implemented")
}

// EnqueueWrite adds a write operation to the queue. Returns false if queue is full.
func (wq *WriteQueue) EnqueueWrite(key string, delta int64) bool {
	select {
	case wq.writeCh <- writeRequest{key: key, delta: delta}:
		return true
	default:
		wq.droppedWrites.Add(1)
		log.Printf("Cache write queue full, dropping write for key %s", key)
		return false
	}
}

// EnqueueSet adds a set value operation to the queue
func (wq *WriteQueue) EnqueueSet(key string, value int64) bool {
	select {
	case wq.writeCh <- writeRequest{key: key, setValue: &value}:
		return true
	default:
		wq.droppedWrites.Add(1)
		log.Printf("Cache write queue full, dropping set for key %s", key)
		return false
	}
}

// Shutdown gracefully shuts down the write queue, waiting for pending writes
func (wq *WriteQueue) Shutdown() {
	close(wq.shutdownCh)
	wq.wg.Wait()
	log.Printf("Cache write queue shutdown complete")
}

// Stats returns write queue statistics
func (wq *WriteQueue) Stats() WriteQueueStats {
	return WriteQueueStats{
		Queued:       len(wq.writeCh),
		DroppedWrites: wq.droppedWrites.Load(),
	}
}

// WriteQueueStats represents write queue statistics
type WriteQueueStats struct {
	Queued       int
	DroppedWrites int64
}

// extractCounterID extracts the counter ID from a cache key
func extractCounterID(key string) string {
	// Key format: tenant_id:counter_id
	// We need to extract just the counter_id part
	if idx := indexLast(key, ':'); idx != -1 {
		return key[idx+1:]
	}
	return key
}

// indexLast finds the last index of a byte in a string
func indexLast(s string, sep byte) int {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == sep {
			return i
		}
	}
	return -1
}

// CachedCounter combines LRU cache with async write queue for high-performance counter operations
type CachedCounter struct {
	cache      *LRUCache
	writeQueue *WriteQueue
	fetchFunc  FetchFunc
	workers    int
	queueSize  int
	started    atomic.Bool
	ctx        context.Context
	cancel     context.CancelFunc
}

// FetchFunc is the function type for fetching a counter from the database
type FetchFunc func(key string) (*models.Counter, error)

// NewCachedCounter creates a new cached counter instance
func NewCachedCounter(cache *LRUCache, fetchFunc FetchFunc, writeFunc WriteFunc, workers, queueSize int) *CachedCounter {
	writeQueue := NewWriteQueue(cache, nil, workers, queueSize)
	writeQueue.writeFunc = writeFunc

	return &CachedCounter{
		cache:      cache,
		writeQueue: writeQueue,
		fetchFunc:  fetchFunc,
		workers:    workers,
		queueSize:  queueSize,
	}
}

// Start starts the background workers
func (cc *CachedCounter) Start(ctx context.Context) {
	if cc.started.CompareAndSwap(false, true) {
		cc.ctx, cc.cancel = context.WithCancel(ctx)
		cc.writeQueue.Start(cc.ctx)
		log.Printf("Cached counter started with %d workers", cc.workers)
	}
}

// Shutdown gracefully shuts down the cached counter
func (cc *CachedCounter) Shutdown() {
	if cc.started.CompareAndSwap(true, false) {
		cc.cancel()
		cc.writeQueue.Shutdown()
		log.Printf("Cached counter shutdown complete")
	}
}

// Get retrieves a counter, using cache if available
func (cc *CachedCounter) Get(tenantID, counterID string) (*models.Counter, error) {
	key := makeCacheKey(tenantID, counterID)

	// Try cache first
	if counter, ok := cc.cache.Get(key); ok {
		return counter, nil
	}

	// Cache miss - fetch from database
	counter, err := cc.fetchFunc(key)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch counter from database: %w", err)
	}

	// Store in cache
	cc.cache.Put(key, counter)

	return counter, nil
}

// IncrementAsync increments a counter asynchronously
func (cc *CachedCounter) IncrementAsync(tenantID, counterID string, delta int64) (int64, error) {
	key := makeCacheKey(tenantID, counterID)

	// Check cache first
	counter, ok := cc.cache.Get(key)
	if ok {
		// Update cache immediately
		newValue := counter.Value + delta
		counter.Value = newValue
		cc.cache.Put(key, counter)

		// Enqueue async write to database
		cc.writeQueue.EnqueueWrite(key, delta)

		return newValue, nil
	}

	// Not in cache - fetch from DB, increment, then cache
	fetchedCounter, err := cc.fetchFunc(key)
	if err != nil {
		return 0, fmt.Errorf("failed to fetch counter: %w", err)
	}

	newValue := fetchedCounter.Value + delta
	fetchedCounter.Value = newValue

	// Store in cache
	cc.cache.Put(key, fetchedCounter)

	// Enqueue async write to database
	cc.writeQueue.EnqueueWrite(key, delta)

	return newValue, nil
}

// SetAsync sets a counter value asynchronously
func (cc *CachedCounter) SetAsync(tenantID, counterID string, value int64) error {
	key := makeCacheKey(tenantID, counterID)

	// Update cache immediately
	counter := &models.Counter{
		TenantID: tenantID,
		ID:       counterID,
		Value:    value,
	}
	cc.cache.Put(key, counter)

	// Enqueue async write to database
	cc.writeQueue.EnqueueSet(key, value)

	return nil
}

// Invalidate removes a counter from the cache
func (cc *CachedCounter) Invalidate(tenantID, counterID string) {
	key := makeCacheKey(tenantID, counterID)
	cc.cache.Delete(key)
}

// Stats returns combined cache and write queue statistics
func (cc *CachedCounter) Stats() CachedCounterStats {
	return CachedCounterStats{
		CacheStats:   cc.cache.Stats(),
		QueueStats:   cc.writeQueue.Stats(),
	}
}

// CachedCounterStats represents combined statistics
type CachedCounterStats struct {
	CacheStats CacheStats
	QueueStats WriteQueueStats
}

// makeCacheKey creates a cache key from tenant ID and counter ID
func makeCacheKey(tenantID, counterID string) string {
	return fmt.Sprintf("%s:%s", tenantID, counterID)
}
