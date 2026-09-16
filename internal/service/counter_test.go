package service

import (
	"context"
	"counter/internal/models"
	"counter/internal/store"
	"errors"
	"testing"
	"time"
)

type fakeCounterRepository struct {
	tenantExists    bool
	counter         *models.Counter
	incrementValue  int64
	incrementErr    error
	incrementTenant string
	incrementID     string
	incrementDelta  int64
	incrementAt     time.Time
	setErr          error
	setTenant       string
	setID           string
	setValue        int64
	setAt           time.Time
}

func (f *fakeCounterRepository) TenantExists(context.Context, string) (bool, error) {
	return f.tenantExists, nil
}

func (f *fakeCounterRepository) CreateCounter(context.Context, *models.Counter) error {
	return nil
}

func (f *fakeCounterRepository) GetCounter(context.Context, string, string) (*models.Counter, error) {
	if f.counter == nil {
		return nil, store.ErrCounterNotFound
	}
	return f.counter, nil
}

func (f *fakeCounterRepository) ListCounters(context.Context, string, *models.CounterCursor, int) ([]models.Counter, error) {
	return nil, nil
}

func (f *fakeCounterRepository) IncrementCounter(_ context.Context, tenantID, counterID string, delta int64, now time.Time) (int64, error) {
	f.incrementTenant = tenantID
	f.incrementID = counterID
	f.incrementDelta = delta
	f.incrementAt = now
	return f.incrementValue, f.incrementErr
}

func (f *fakeCounterRepository) SetCounterValue(_ context.Context, tenantID, counterID string, value int64, now time.Time) error {
	f.setTenant = tenantID
	f.setID = counterID
	f.setValue = value
	f.setAt = now
	return f.setErr
}

func TestCounterServiceIncrementEnforcesMaxDelta(t *testing.T) {
	repository := &fakeCounterRepository{counter: &models.Counter{MaxDelta: 5}}
	service := NewCounterService(repository)

	_, err := service.Increment(context.Background(), "tenant-id", "counter-id", 6)
	if !errors.Is(err, ErrDeltaExceedsMaximum) {
		t.Fatalf("increment error = %v, want %v", err, ErrDeltaExceedsMaximum)
	}
	if repository.incrementDelta != 0 {
		t.Fatal("repository was called after max-delta rejection")
	}
}

func TestCounterServiceMutationsPassOwnershipAndTimestamp(t *testing.T) {
	now := time.Date(2026, time.January, 3, 4, 5, 6, 0, time.UTC)
	repository := &fakeCounterRepository{
		counter:        &models.Counter{MaxDelta: 10},
		incrementValue: 12,
	}
	service := NewCounterServiceWithDependencies(repository, func() time.Time { return now }, func() string { return "unused" })

	increment, err := service.Increment(context.Background(), "tenant-id", "counter-id", 4)
	if err != nil {
		t.Fatalf("increment: %v", err)
	}
	if repository.incrementTenant != "tenant-id" || repository.incrementID != "counter-id" || repository.incrementDelta != 4 || !repository.incrementAt.Equal(now) {
		t.Fatalf("increment repository call = %q/%q/%d/%v", repository.incrementTenant, repository.incrementID, repository.incrementDelta, repository.incrementAt)
	}
	if increment.Value != 12 || !increment.UpdatedAt.Equal(now) {
		t.Fatalf("increment result = %#v", increment)
	}

	set, err := service.Set(context.Background(), "tenant-id", "counter-id", 42)
	if err != nil {
		t.Fatalf("set: %v", err)
	}
	if repository.setTenant != "tenant-id" || repository.setID != "counter-id" || repository.setValue != 42 || !repository.setAt.Equal(now) {
		t.Fatalf("set repository call = %q/%q/%d/%v", repository.setTenant, repository.setID, repository.setValue, repository.setAt)
	}
	if set.Value != 42 || !set.UpdatedAt.Equal(now) {
		t.Fatalf("set result = %#v", set)
	}
}
