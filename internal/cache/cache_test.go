package cache

import (
	"context"
	"counter/internal/database"
	"counter/internal/models"
	"sync"
	"testing"
	"time"
)

func TestLRUCacheBasicOperations(t *testing.T) {
	cache := NewLRUCache(2)

	// Test put and get
	counter := &models.Counter{
		ID:       "counter1",
		TenantID: "tenant1",
		Label:    "likes",
		Value:    42,
	}

	cache.Put("tenant1:counter1", counter)

	// Should retrieve the same counter
	retrieved, ok := cache.Get("tenant1:counter1")
	if !ok {
		t.Fatal("Expected to find counter in cache")
	}

	if retrieved.ID != "counter1" || retrieved.Value != 42 {
		t.Errorf("Expected counter1 with value 42, got %+v", retrieved)
	}

	// Test miss
	_, ok = cache.Get("tenant2:counter2")
	if ok {
		t.Error("Expected cache miss for non-existent key")
	}
}

func TestLRUCacheEviction(t *testing.T) {
	cache := NewLRUCache(2)

	// Add 3 counters to a cache of size 2
	cache.Put("tenant1:counter1", &models.Counter{ID: "counter1", Value: 1})
	cache.Put("tenant2:counter2", &models.Counter{ID: "counter2", Value: 2})
	cache.Put("tenant3:counter3", &models.Counter{ID: "counter3", Value: 3})

	// First counter should be evicted
	_, ok := cache.Get("tenant1:counter1")
	if ok {
		t.Error("Expected counter1 to be evicted from cache")
	}

	// Second and third should still be there
	_, ok = cache.Get("tenant2:counter2")
	if !ok {
		t.Error("Expected counter2 to be in cache")
	}

	_, ok = cache.Get("tenant3:counter3")
	if !ok {
		t.Error("Expected counter3 to be in cache")
	}
}

func TestLRUCacheGetUpdatesOrder(t *testing.T) {
	cache := NewLRUCache(2)

	cache.Put("tenant1:counter1", &models.Counter{ID: "counter1", Value: 1})
	cache.Put("tenant2:counter2", &models.Counter{ID: "counter2", Value: 2})

	// Access counter1 to make it recently used
	cache.Get("tenant1:counter1")

	// Add counter3 - should evict counter2 (least recently used)
	cache.Put("tenant3:counter3", &models.Counter{ID: "counter3", Value: 3})

	_, ok := cache.Get("tenant1:counter1")
	if !ok {
		t.Error("Expected counter1 to still be in cache after being accessed")
	}

	_, ok = cache.Get("tenant2:counter2")
	if ok {
		t.Error("Expected counter2 to be evicted (least recently used)")
	}

	_, ok = cache.Get("tenant3:counter3")
	if !ok {
		t.Error("Expected counter3 to be in cache")
	}
}

func TestWriteQueueAsyncProcessing(t *testing.T) {
	// Create a mock database that tracks writes
	var writeMutex sync.Mutex
	var writes []struct {
		counterID string
		delta     int64
	}

	mockDB := &database.DB{} // We'll mock the actual write operation

	cache := NewLRUCache(10)
	writeQueue := NewWriteQueue(cache, mockDB, 2, 100) // 2 workers, queue size 100

	// Mock the write function
	writeQueue.writeFunc = func(counterID string, delta int64) error {
		writeMutex.Lock()
		defer writeMutex.Unlock()
		writes = append(writes, struct {
			counterID string
			delta     int64
		}{counterID, delta})
		return nil
	}

	// Start the workers
	ctx := context.Background()
	writeQueue.Start(ctx)
	defer writeQueue.Shutdown()

	// Enqueue some writes
	writeQueue.EnqueueWrite("tenant1:counter1", 5)
	writeQueue.EnqueueWrite("tenant1:counter1", 3)
	writeQueue.EnqueueWrite("tenant2:counter2", 10)

	// Wait a bit for async processing
	time.Sleep(100 * time.Millisecond)

	writeMutex.Lock()
	writeCount := len(writes)
	writeMutex.Unlock()

	if writeCount != 3 {
		t.Errorf("Expected 3 writes, got %d", writeCount)
	}
}

func TestWriteQueueShutdownWaitsForWrites(t *testing.T) {
	var writeStarted sync.WaitGroup
	var writeCompleted sync.WaitGroup

	mockDB := &database.DB{}

	cache := NewLRUCache(10)
	writeQueue := NewWriteQueue(cache, mockDB, 1, 100)

	// Mock a slow write function
	writeQueue.writeFunc = func(counterID string, delta int64) error {
		writeStarted.Done()
		time.Sleep(50 * time.Millisecond)
		writeCompleted.Done()
		return nil
	}

	ctx := context.Background()
	writeQueue.Start(ctx)

	// Enqueue a write and wait for it to start
	writeStarted.Add(1)
	writeCompleted.Add(1)
	writeQueue.EnqueueWrite("tenant1:counter1", 5)

	writeStarted.Wait()

	// Shutdown should wait for the write to complete
	start := time.Now()
	writeQueue.Shutdown()
	duration := time.Since(start)

	if duration < 50*time.Millisecond {
		t.Errorf("Expected shutdown to wait for writes, but it only took %v", duration)
	}

	writeCompleted.Wait()
}

func TestCachedCounterGet(t *testing.T) {
	cache := NewLRUCache(10)

	// Mock database fetch function
	var dbCallCount int
	mockFetch := func(key string) (*models.Counter, error) {
		dbCallCount++
		return &models.Counter{
			ID:       "counter1",
			TenantID: "tenant1",
			Value:    42,
		}, nil
	}

	cachedCache := NewCachedCounter(cache, mockFetch, nil, 1, 100)

	// First call should hit database
	counter, err := cachedCache.Get("tenant1", "counter1")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if dbCallCount != 1 {
		t.Errorf("Expected 1 DB call, got %d", dbCallCount)
	}

	if counter.Value != 42 {
		t.Errorf("Expected value 42, got %d", counter.Value)
	}

	// Second call should use cache
	counter, err = cachedCache.Get("tenant1", "counter1")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if dbCallCount != 1 {
		t.Errorf("Expected second call to use cache, but got %d DB calls", dbCallCount)
	}
}

func TestCachedCounterIncrementAsync(t *testing.T) {
	cache := NewLRUCache(10)

	var dbWriteDelta int64
	var dbWriteMutex sync.Mutex

	mockFetch := func(key string) (*models.Counter, error) {
		return &models.Counter{
			ID:       "counter1",
			TenantID: "tenant1",
			Value:    100,
		}, nil
	}

	mockWrite := func(counterID string, delta int64) error {
		dbWriteMutex.Lock()
		defer dbWriteMutex.Unlock()
		dbWriteDelta = delta
		return nil
	}

	cachedCache := NewCachedCounter(cache, mockFetch, mockWrite, 1, 100)

	ctx := context.Background()
	cachedCache.Start(ctx)
	defer cachedCache.Shutdown()

	// First increment - not in cache, should fetch from DB
	newValue, err := cachedCache.IncrementAsync("tenant1", "counter1", 5)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if newValue != 105 {
		t.Errorf("Expected value 105, got %d", newValue)
	}

	// Wait for async write
	time.Sleep(50 * time.Millisecond)

	dbWriteMutex.Lock()
	if dbWriteDelta != 5 {
		t.Errorf("Expected DB write delta of 5, got %d", dbWriteDelta)
	}
	dbWriteMutex.Unlock()

	// Second increment - should use cache
	newValue, err = cachedCache.IncrementAsync("tenant1", "counter1", 3)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if newValue != 108 {
		t.Errorf("Expected value 108, got %d", newValue)
	}

	// Verify cache was updated
	counter, ok := cache.Get("tenant1:counter1")
	if !ok {
		t.Fatal("Expected counter to be in cache")
	}

	if counter.Value != 108 {
		t.Errorf("Expected cached value 108, got %d", counter.Value)
	}
}
