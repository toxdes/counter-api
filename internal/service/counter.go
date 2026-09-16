package service

import (
	"context"
	"counter/internal/models"
	"counter/internal/store"
	"crypto/sha256"
	"encoding/binary"
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

type CounterMutationResult struct {
	OperationID string
	CounterID   string
	Delta       int64
	Value       int64
	UpdatedAt   time.Time
	Replayed    bool
}

// CounterRepository is the persistence boundary required by CounterService.
type CounterRepository interface {
	TenantExists(context.Context, string) (bool, error)
	CreateCounter(context.Context, *models.Counter) error
	GetCounter(context.Context, string, string) (*models.Counter, error)
	ListCounters(context.Context, string, *CounterCursor, int) ([]models.Counter, error)
	IncrementCounter(context.Context, string, string, int64, string, []byte, time.Time) (store.MutationResult, error)
	SetCounterValueWithOperation(context.Context, string, string, int64, string, []byte, time.Time) (store.MutationResult, error)
}

// CounterService contains counter read and creation use cases independent of
// HTTP and SQL.
type CounterService interface {
	Create(context.Context, string, models.CreateCounterRequest) (*models.Counter, error)
	Get(context.Context, string, string) (*models.Counter, error)
	List(context.Context, string, *CounterCursor, int) (CounterPage, error)
	Increment(context.Context, string, string, int64) (*CounterMutationResult, error)
	IncrementWithOperation(context.Context, string, string, int64, string) (*CounterMutationResult, error)
	Set(context.Context, string, string, int64) (*CounterMutationResult, error)
	SetWithOperation(context.Context, string, string, int64, string) (*CounterMutationResult, error)
}

// AttributableCounterService is implemented by the durable service to attach
// the authenticated principal to operation-history records.
type AttributableCounterService interface {
	CounterService
	IncrementWithActor(context.Context, string, string, int64, string) (*CounterMutationResult, error)
	IncrementWithOperationActor(context.Context, string, string, int64, string, string) (*CounterMutationResult, error)
	SetWithActor(context.Context, string, string, int64, string) (*CounterMutationResult, error)
	SetWithOperationActor(context.Context, string, string, int64, string, string) (*CounterMutationResult, error)
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

func (s *counterService) Increment(ctx context.Context, tenantID, counterID string, delta int64) (*CounterMutationResult, error) {
	return s.IncrementWithOperation(ctx, tenantID, counterID, delta, "")
}

func (s *counterService) IncrementWithOperation(ctx context.Context, tenantID, counterID string, delta int64, operationID string) (*CounterMutationResult, error) {
	return s.incrementWithActor(ctx, tenantID, counterID, delta, operationID, "")
}

func (s *counterService) IncrementWithActor(ctx context.Context, tenantID, counterID string, delta int64, actorID string) (*CounterMutationResult, error) {
	return s.incrementWithActor(ctx, tenantID, counterID, delta, "", actorID)
}

func (s *counterService) IncrementWithOperationActor(ctx context.Context, tenantID, counterID string, delta int64, operationID, actorID string) (*CounterMutationResult, error) {
	return s.incrementWithActor(ctx, tenantID, counterID, delta, operationID, actorID)
}

func (s *counterService) incrementWithActor(ctx context.Context, tenantID, counterID string, delta int64, operationID, actorID string) (*CounterMutationResult, error) {
	if operationID == "" {
		operationID = s.newID()
	}

	now := s.now()
	result, err := s.incrementRepository(ctx, tenantID, counterID, delta, operationID, incrementRequestHash(tenantID, counterID, delta), actorID, now)
	if errors.Is(err, store.ErrCounterNotFound) {
		return nil, ErrCounterNotFound
	}
	if errors.Is(err, store.ErrDeltaExceedsMaximum) {
		return nil, ErrDeltaExceedsMaximum
	}
	if errors.Is(err, store.ErrCounterOverflow) {
		return nil, ErrCounterOverflow
	}
	if errors.Is(err, store.ErrIdempotencyKeyReused) {
		return nil, ErrIdempotencyKeyReused
	}
	if errors.Is(err, store.ErrOperationInProgress) {
		return nil, ErrOperationInProgress
	}
	if err != nil {
		return nil, err
	}
	return &CounterMutationResult{
		OperationID: result.OperationID,
		CounterID:   counterID,
		Delta:       result.Delta,
		Value:       result.Value,
		UpdatedAt:   result.UpdatedAt,
		Replayed:    result.Replayed,
	}, nil
}

func (s *counterService) Set(ctx context.Context, tenantID, counterID string, value int64) (*CounterMutationResult, error) {
	return s.SetWithOperation(ctx, tenantID, counterID, value, "")
}

func (s *counterService) SetWithOperation(ctx context.Context, tenantID, counterID string, value int64, operationID string) (*CounterMutationResult, error) {
	return s.setWithActor(ctx, tenantID, counterID, value, operationID, "")
}

func (s *counterService) SetWithActor(ctx context.Context, tenantID, counterID string, value int64, actorID string) (*CounterMutationResult, error) {
	return s.setWithActor(ctx, tenantID, counterID, value, "", actorID)
}

func (s *counterService) SetWithOperationActor(ctx context.Context, tenantID, counterID string, value int64, operationID, actorID string) (*CounterMutationResult, error) {
	return s.setWithActor(ctx, tenantID, counterID, value, operationID, actorID)
}

func (s *counterService) setWithActor(ctx context.Context, tenantID, counterID string, value int64, operationID, actorID string) (*CounterMutationResult, error) {
	if operationID == "" {
		operationID = s.newID()
	}

	now := s.now()
	result, err := s.setRepository(ctx, tenantID, counterID, value, operationID, setRequestHash(tenantID, counterID, value), actorID, now)
	if errors.Is(err, store.ErrCounterNotFound) {
		return nil, ErrCounterNotFound
	}
	if errors.Is(err, store.ErrIdempotencyKeyReused) {
		return nil, ErrIdempotencyKeyReused
	}
	if errors.Is(err, store.ErrOperationInProgress) {
		return nil, ErrOperationInProgress
	}
	if errors.Is(err, store.ErrCounterOverflow) {
		return nil, ErrCounterOverflow
	}
	if err != nil {
		return nil, err
	}
	return &CounterMutationResult{
		OperationID: result.OperationID,
		CounterID:   counterID,
		Delta:       result.Delta,
		Value:       result.Value,
		UpdatedAt:   result.UpdatedAt,
		Replayed:    result.Replayed,
	}, nil
}

func (s *counterService) incrementRepository(ctx context.Context, tenantID, counterID string, delta int64, operationID string, requestHash []byte, actorID string, now time.Time) (store.MutationResult, error) {
	if actorRepository, ok := s.repository.(interface {
		IncrementCounterWithActor(context.Context, string, string, int64, string, []byte, string, time.Time) (store.MutationResult, error)
	}); ok {
		return actorRepository.IncrementCounterWithActor(ctx, tenantID, counterID, delta, operationID, requestHash, actorID, now)
	}
	return s.repository.IncrementCounter(ctx, tenantID, counterID, delta, operationID, requestHash, now)
}

func (s *counterService) setRepository(ctx context.Context, tenantID, counterID string, value int64, operationID string, requestHash []byte, actorID string, now time.Time) (store.MutationResult, error) {
	if actorRepository, ok := s.repository.(interface {
		SetCounterValueWithOperationWithActor(context.Context, string, string, int64, string, []byte, string, time.Time) (store.MutationResult, error)
	}); ok {
		return actorRepository.SetCounterValueWithOperationWithActor(ctx, tenantID, counterID, value, operationID, requestHash, actorID, now)
	}
	return s.repository.SetCounterValueWithOperation(ctx, tenantID, counterID, value, operationID, requestHash, now)
}

func incrementRequestHash(tenantID, counterID string, delta int64) []byte {
	return mutationRequestHash("increment", tenantID, counterID, delta)
}

func setRequestHash(tenantID, counterID string, value int64) []byte {
	return mutationRequestHash("set", tenantID, counterID, value)
}

func mutationRequestHash(kind, tenantID, counterID string, value int64) []byte {
	hash := sha256.New()
	for _, field := range []string{"counter-operation-v1", kind, tenantID, counterID} {
		var length [8]byte
		binary.BigEndian.PutUint64(length[:], uint64(len(field)))
		hash.Write(length[:])
		hash.Write([]byte(field))
	}
	var encoded [8]byte
	binary.BigEndian.PutUint64(encoded[:], uint64(value))
	hash.Write(encoded[:])
	return hash.Sum(nil)
}
