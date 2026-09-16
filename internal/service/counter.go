package service

import (
	"context"
	"counter/internal/models"
	"counter/internal/store"
	"errors"
	"time"

	"github.com/google/uuid"
)

// CounterCursor is kept as a service-level alias for callers that do not need
// to depend on transport-specific cursor encoding.
type CounterCursor = models.CounterCursor

type CounterPage struct {
	Counters []models.Counter
	Next     *CounterCursor
}

// CounterRepository is the persistence boundary required by CounterService.
type CounterRepository interface {
	TenantExists(context.Context, string) (bool, error)
	CreateCounter(context.Context, *models.Counter) error
	GetCounter(context.Context, string, string) (*models.Counter, error)
	ListCounters(context.Context, string, *CounterCursor, int) ([]models.Counter, error)
}

// CounterService contains counter read and creation use cases independent of
// HTTP and SQL.
type CounterService interface {
	Create(context.Context, string, models.CreateCounterRequest) (*models.Counter, error)
	Get(context.Context, string, string) (*models.Counter, error)
	List(context.Context, string, *CounterCursor, int) (CounterPage, error)
}

type counterService struct {
	repository CounterRepository
	now        func() time.Time
	newID      func() string
}

func NewCounterService(repository CounterRepository) CounterService {
	return &counterService{
		repository: repository,
		now:        func() time.Time { return time.Now().UTC() },
		newID:      func() string { return uuid.NewString() },
	}
}

func NewCounterServiceWithDependencies(repository CounterRepository, now func() time.Time, newID func() string) CounterService {
	return &counterService{repository: repository, now: now, newID: newID}
}

func (s *counterService) Create(ctx context.Context, tenantID string, request models.CreateCounterRequest) (*models.Counter, error) {
	exists, err := s.repository.TenantExists(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, ErrTenantNotFound
	}

	now := s.now()
	counter := &models.Counter{
		ID:        s.newID(),
		TenantID:  tenantID,
		Label:     request.Label,
		Value:     request.InitialValue,
		MaxDelta:  request.MaxDelta,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := s.repository.CreateCounter(ctx, counter); err != nil {
		if errors.Is(err, store.ErrCounterConflict) {
			return nil, ErrConflict
		}
		if errors.Is(err, store.ErrTenantNotFound) {
			return nil, ErrTenantNotFound
		}
		return nil, err
	}
	return counter, nil
}

func (s *counterService) Get(ctx context.Context, tenantID, counterID string) (*models.Counter, error) {
	counter, err := s.repository.GetCounter(ctx, tenantID, counterID)
	if errors.Is(err, store.ErrCounterNotFound) {
		return nil, ErrCounterNotFound
	}
	return counter, err
}

func (s *counterService) List(ctx context.Context, tenantID string, cursor *CounterCursor, limit int) (CounterPage, error) {
	exists, err := s.repository.TenantExists(ctx, tenantID)
	if err != nil {
		return CounterPage{}, err
	}
	if !exists {
		return CounterPage{}, ErrTenantNotFound
	}

	counters, err := s.repository.ListCounters(ctx, tenantID, cursor, limit)
	if err != nil {
		return CounterPage{}, err
	}
	page := CounterPage{Counters: counters}
	if len(counters) > limit {
		page.Counters = counters[:limit]
		last := page.Counters[len(page.Counters)-1]
		page.Next = &CounterCursor{CreatedAt: last.CreatedAt, ID: last.ID}
	}
	return page, nil
}
