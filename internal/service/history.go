package service

import (
	"context"
	"counter/internal/models"
	"counter/internal/store"
	"errors"
)

// OperationCursor is the service-level representation of a history cursor.
type OperationCursor = models.OperationCursor

type OperationHistoryPage struct {
	Operations []models.CounterOperation
	Next       *OperationCursor
}

// OperationHistoryRepository is the persistence boundary for history reads.
type OperationHistoryRepository interface {
	GetCounter(context.Context, string, string) (*models.Counter, error)
	ListOperations(context.Context, string, string, *OperationCursor, int) ([]models.CounterOperation, error)
}

// OperationHistoryService provides tenant-scoped operation history.
type OperationHistoryService interface {
	List(context.Context, string, string, *OperationCursor, int) (OperationHistoryPage, error)
}

type operationHistoryService struct {
	repository OperationHistoryRepository
}

func NewOperationHistoryService(repository OperationHistoryRepository) OperationHistoryService {
	return &operationHistoryService{repository: repository}
}

func (s *operationHistoryService) List(ctx context.Context, tenantID, counterID string, cursor *OperationCursor, limit int) (OperationHistoryPage, error) {
	if _, err := s.repository.GetCounter(ctx, tenantID, counterID); errors.Is(err, store.ErrCounterNotFound) {
		return OperationHistoryPage{}, ErrCounterNotFound
	} else if err != nil {
		return OperationHistoryPage{}, err
	}

	operations, err := s.repository.ListOperations(ctx, tenantID, counterID, cursor, limit)
	if err != nil {
		return OperationHistoryPage{}, err
	}

	page := OperationHistoryPage{Operations: operations}
	if len(operations) > limit {
		page.Operations = operations[:limit]
		last := page.Operations[len(page.Operations)-1]
		page.Next = &OperationCursor{CreatedAt: last.CreatedAt, OperationID: last.OperationID}
	}
	return page, nil
}

// ReconciliationRepository is the persistence boundary for bounded checks.
type ReconciliationRepository interface {
	Reconcile(context.Context, int) (models.ReconciliationReport, error)
}

// ReconciliationService runs operator-facing consistency checks.
type ReconciliationService interface {
	Reconcile(context.Context, int) (models.ReconciliationReport, error)
}

type reconciliationService struct {
	repository ReconciliationRepository
}

func NewReconciliationService(repository ReconciliationRepository) ReconciliationService {
	return &reconciliationService{repository: repository}
}

func (s *reconciliationService) Reconcile(ctx context.Context, batchSize int) (models.ReconciliationReport, error) {
	return s.repository.Reconcile(ctx, batchSize)
}
