package service

import (
	"context"
	"counter/internal/models"
	"errors"
	"testing"
)

type countingCounterRepository struct {
	*fakeCounterRepository
	getCalls int
}

func (r *countingCounterRepository) GetCounter(ctx context.Context, tenantID, counterID string) (*models.Counter, error) {
	r.getCalls++
	return r.fakeCounterRepository.GetCounter(ctx, tenantID, counterID)
}

type testCounterReadCache struct {
	entries   map[string]*models.Counter
	getErr    error
	setErr    error
	deleteErr error
	getCalls  int
	setCalls  int
	deletes   int
}

func newTestCounterReadCache() *testCounterReadCache {
	return &testCounterReadCache{entries: make(map[string]*models.Counter)}
}

func (c *testCounterReadCache) Get(_ context.Context, tenantID, counterID string) (*models.Counter, bool, error) {
	c.getCalls++
	if c.getErr != nil {
		return nil, false, c.getErr
	}
	counter, ok := c.entries[tenantID+":"+counterID]
	return counter, ok, nil
}

func (c *testCounterReadCache) Set(_ context.Context, counter *models.Counter) error {
	c.setCalls++
	if c.setErr != nil {
		return c.setErr
	}
	c.entries[counter.TenantID+":"+counter.ID] = counter
	return nil
}

func (c *testCounterReadCache) Delete(_ context.Context, tenantID, counterID string) error {
	c.deletes++
	if c.deleteErr != nil {
		return c.deleteErr
	}
	delete(c.entries, tenantID+":"+counterID)
	return nil
}

func TestCounterServiceReadCacheMissThenHit(t *testing.T) {
	repository := &countingCounterRepository{fakeCounterRepository: &fakeCounterRepository{
		counter: &models.Counter{ID: "counter-id", TenantID: "tenant-id", Value: 123},
	}}
	readCache := newTestCounterReadCache()
	service := NewCounterServiceWithReadCache(repository, readCache)

	for i := 0; i < 2; i++ {
		counter, err := service.Get(context.Background(), "tenant-id", "counter-id")
		if err != nil || counter.Value != 123 {
			t.Fatalf("Get() = (%#v, %v), want value 123", counter, err)
		}
	}
	if repository.getCalls != 1 || readCache.setCalls != 1 {
		t.Fatalf("database gets=%d cache sets=%d, want 1 each", repository.getCalls, readCache.setCalls)
	}
}

func TestCounterServiceInvalidatesReadCacheAfterSuccessfulMutation(t *testing.T) {
	repository := &countingCounterRepository{fakeCounterRepository: &fakeCounterRepository{
		counter:        &models.Counter{ID: "counter-id", TenantID: "tenant-id", Value: 10},
		incrementValue: 11,
	}}
	readCache := newTestCounterReadCache()
	readCache.deleteErr = errors.New("cache unavailable")
	service := NewCounterServiceWithReadCache(repository, readCache)
	if _, err := service.Get(context.Background(), "tenant-id", "counter-id"); err != nil {
		t.Fatalf("prime cache: %v", err)
	}

	if _, err := service.Increment(context.Background(), "tenant-id", "counter-id", 1); err != nil {
		t.Fatalf("Increment(): %v", err)
	}
	if readCache.deletes != 1 {
		t.Fatalf("cache invalidations = %d, want 1", readCache.deletes)
	}
	readCache.deleteErr = nil
	if _, err := service.Set(context.Background(), "tenant-id", "counter-id", 20); err != nil {
		t.Fatalf("Set(): %v", err)
	}
	if readCache.deletes != 2 {
		t.Fatalf("cache invalidations after set = %d, want 2", readCache.deletes)
	}
}

func TestCounterServiceReadCacheFailuresFallBackToDatabase(t *testing.T) {
	repository := &countingCounterRepository{fakeCounterRepository: &fakeCounterRepository{
		counter: &models.Counter{ID: "counter-id", TenantID: "tenant-id", Value: 123},
	}}
	readCache := newTestCounterReadCache()
	readCache.getErr = errors.New("cache unavailable")
	readCache.setErr = errors.New("cache unavailable")
	service := NewCounterServiceWithReadCache(repository, readCache)

	counter, err := service.Get(context.Background(), "tenant-id", "counter-id")
	if err != nil || counter.Value != 123 {
		t.Fatalf("Get() = (%#v, %v), want database value 123", counter, err)
	}
	if repository.getCalls != 1 {
		t.Fatalf("database gets = %d, want 1", repository.getCalls)
	}
}
