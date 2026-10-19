package service

import (
	"bytes"
	"context"
	"counter/internal/models"
	"counter/internal/store"
	"encoding/json"
	"errors"
	"io"
)

const MaxMetadataBytes = 64 * 1024
const MaxMetadataDepth = 64

var ErrMetadataTooLarge = errors.New("metadata exceeds 64 KiB")
var ErrInvalidMetadata = store.ErrInvalidMetadata
var ErrMetadataNotFound = store.ErrMetadataNotFound
var ErrMetadataPrecondition = store.ErrMetadataPrecondition

type MetadataRepository interface {
	CounterExists(context.Context, string, string) (bool, error)
	Get(context.Context, string, string) (*models.CounterMetadata, error)
	Put(context.Context, string, string, json.RawMessage, string, bool) (*models.CounterMetadata, error)
	Delete(context.Context, string, string, string) error
}

type MetadataService struct{ repository MetadataRepository }

func NewMetadataService(repository MetadataRepository) *MetadataService {
	return &MetadataService{repository: repository}
}

func ValidateMetadata(data json.RawMessage) error {
	if len(data) > MaxMetadataBytes {
		return ErrMetadataTooLarge
	}
	if !json.Valid(data) {
		return ErrInvalidMetadata
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	depth := 0
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return ErrInvalidMetadata
		}
		if delimiter, ok := token.(json.Delim); ok {
			if delimiter == '{' || delimiter == '[' {
				depth++
			} else {
				depth--
			}
			if depth > MaxMetadataDepth {
				return ErrInvalidMetadata
			}
		}
	}
	return nil
}

func (s *MetadataService) Get(ctx context.Context, tenantID, counterID string) (*models.CounterMetadata, error) {
	exists, err := s.repository.CounterExists(ctx, tenantID, counterID)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, ErrCounterNotFound
	}
	return s.repository.Get(ctx, tenantID, counterID)
}

func (s *MetadataService) Put(ctx context.Context, tenantID, counterID string, data json.RawMessage, expected string, create bool) (*models.CounterMetadata, error) {
	if err := ValidateMetadata(data); err != nil {
		return nil, err
	}
	result, err := s.repository.Put(ctx, tenantID, counterID, data, expected, create)
	if errors.Is(err, store.ErrCounterNotFound) {
		return nil, ErrCounterNotFound
	}
	return result, err
}

func (s *MetadataService) Delete(ctx context.Context, tenantID, counterID, expected string) error {
	err := s.repository.Delete(ctx, tenantID, counterID, expected)
	if errors.Is(err, store.ErrCounterNotFound) {
		return ErrCounterNotFound
	}
	return err
}
