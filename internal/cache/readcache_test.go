package cache

import (
	"context"
	"counter/internal/models"
	"fmt"
	"os"
	"testing"
	"time"
)

func TestMemoryCounterReadCacheExpiresAndReturnsCopies(t *testing.T) {
	readCache, err := NewMemoryCounterReadCache(2, 20*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	counter := &models.Counter{ID: "counter-1", TenantID: "tenant-1", Value: 7}
	if err := readCache.Set(context.Background(), counter); err != nil {
		t.Fatal(err)
	}
	counter.Value = 99

	cached, found, err := readCache.Get(context.Background(), "tenant-1", "counter-1")
	if err != nil || !found || cached.Value != 7 {
		t.Fatalf("Get() = (%#v, %v, %v), want cached copy with value 7", cached, found, err)
	}
	time.Sleep(30 * time.Millisecond)
	if _, found, err := readCache.Get(context.Background(), "tenant-1", "counter-1"); err != nil || found {
		t.Fatalf("expired Get() found=%v err=%v, want miss", found, err)
	}
}

func TestMemoryCounterReadCacheEvictsLeastRecentlyUsedEntry(t *testing.T) {
	readCache, err := NewMemoryCounterReadCache(2, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	for _, counter := range []*models.Counter{
		{ID: "counter-1", TenantID: "tenant-1", Value: 1},
		{ID: "counter-2", TenantID: "tenant-1", Value: 2},
	} {
		if err := readCache.Set(ctx, counter); err != nil {
			t.Fatal(err)
		}
	}
	if _, found, err := readCache.Get(ctx, "tenant-1", "counter-1"); err != nil || !found {
		t.Fatalf("touch oldest entry found=%v err=%v", found, err)
	}
	if err := readCache.Set(ctx, &models.Counter{ID: "counter-3", TenantID: "tenant-1", Value: 3}); err != nil {
		t.Fatal(err)
	}
	if _, found, err := readCache.Get(ctx, "tenant-1", "counter-2"); err != nil || found {
		t.Fatalf("least-recently-used entry found=%v err=%v, want eviction", found, err)
	}
}

func TestMemoryCounterReadCacheCanonicalizesUUIDCase(t *testing.T) {
	readCache, err := NewMemoryCounterReadCache(2, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if err := readCache.Set(context.Background(), &models.Counter{ID: "ABC", TenantID: "TENANT", Value: 5}); err != nil {
		t.Fatal(err)
	}
	counter, found, err := readCache.Get(context.Background(), "tenant", "abc")
	if err != nil || !found || counter.Value != 5 {
		t.Fatalf("case-insensitive cache Get() = (%#v, %v, %v)", counter, found, err)
	}
}

func TestRedisCounterReadCacheSharedCapacityAndInvalidation(t *testing.T) {
	redisURL := os.Getenv("COUNTER_READ_CACHE_TEST_URL")
	if redisURL == "" {
		t.Skip("COUNTER_READ_CACHE_TEST_URL is not configured")
	}

	first, err := NewRedisCounterReadCache(redisURL, 2, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	second, err := NewRedisCounterReadCache(redisURL, 2, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()

	ctx := context.Background()
	tenantID := fmt.Sprintf("cache-test-%d", time.Now().UnixNano())
	counters := []*models.Counter{
		{ID: "counter-1", TenantID: tenantID, Value: 1},
		{ID: "counter-2", TenantID: tenantID, Value: 2},
		{ID: "counter-3", TenantID: tenantID, Value: 3},
	}
	defer func() {
		for _, counter := range counters {
			_ = first.Delete(ctx, tenantID, counter.ID)
		}
	}()
	for _, counter := range counters[:2] {
		if err := first.Set(ctx, counter); err != nil {
			t.Fatal(err)
		}
	}
	if _, found, err := second.Get(ctx, tenantID, "counter-1"); err != nil || !found {
		t.Fatalf("shared Redis Get() found=%v err=%v", found, err)
	}
	if err := first.Set(ctx, counters[2]); err != nil {
		t.Fatal(err)
	}
	if _, found, err := second.Get(ctx, tenantID, "counter-2"); err != nil || found {
		t.Fatalf("least-recently-used Redis entry found=%v err=%v, want eviction", found, err)
	}
	if err := second.Delete(ctx, tenantID, "counter-1"); err != nil {
		t.Fatal(err)
	}
	if _, found, err := first.Get(ctx, tenantID, "counter-1"); err != nil || found {
		t.Fatalf("shared Redis invalidation found=%v err=%v, want miss", found, err)
	}
}
