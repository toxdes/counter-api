package models

import (
	"counter/internal/utils"
	"encoding/json"
	"errors"
	"time"
)

// Counter represents a counter in the system
type Counter struct {
	ID        string    `json:"counter_id" db:"id"`
	TenantID  string    `json:"tenant_id" db:"tenant_id"`
	Label     string    `json:"label" db:"label"`
	Value     int64     `json:"value" db:"value"`
	MaxDelta  int64     `json:"max_delta" db:"max_delta"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
	UpdatedAt time.Time `json:"updated_at" db:"updated_at"`
}

// CounterCursor identifies the last counter in a cursor-paginated page.
type CounterCursor struct {
	CreatedAt time.Time
	ID        string
}

// OperationCursor identifies the last operation in a descending history page.
type OperationCursor struct {
	CreatedAt   time.Time
	OperationID string
}

// CounterOperation is a safe, completed operation-history record exposed to
// API clients. Request hashes and other internal persistence fields are not
// exposed.
type CounterOperation struct {
	OperationID string          `json:"operation_id" db:"operation_id"`
	CounterID   string          `json:"counter_id" db:"counter_id"`
	Kind        string          `json:"kind" db:"kind"`
	Delta       int64           `json:"delta" db:"delta"`
	ValueBefore *int64          `json:"value_before,omitempty" db:"value_before"`
	ValueAfter  *int64          `json:"value_after,omitempty" db:"value_after"`
	ActorID     *string         `json:"actor_id,omitempty" db:"actor_id"`
	Metadata    json.RawMessage `json:"metadata" db:"metadata"`
	CreatedAt   time.Time       `json:"created_at" db:"created_at"`
	CompletedAt *time.Time      `json:"completed_at,omitempty" db:"completed_at"`
}

// OperationHistoryResponse is the paginated operation-history response.
type OperationHistoryResponse struct {
	Operations []CounterOperation `json:"operations"`
	NextCursor *string            `json:"next_cursor"`
}

// ReconciliationReport summarizes a bounded operation-history reconciliation.
type ReconciliationReport struct {
	CountersChecked        int64     `json:"counters_checked"`
	OperationsScanned      int64     `json:"operations_scanned"`
	Mismatches             int64     `json:"mismatches"`
	InitialValueViolations int64     `json:"initial_value_violations"`
	StartedAt              time.Time `json:"started_at"`
	CompletedAt            time.Time `json:"completed_at"`
}

// Validate validates the counter data
func (c *Counter) Validate() error {
	if c == nil {
		return errors.New("counter is nil")
	}
	if c.TenantID == "" {
		return errors.New("tenant_id is required")
	}
	if c.Label != "" {
		if err := utils.ValidateLabel(c.Label); err != nil {
			return err
		}
	}
	if err := utils.ValidateCounterValue(c.Value); err != nil {
		return err
	}
	return nil
}

// CreateCounterRequest represents a request to create a counter
type CreateCounterRequest struct {
	Label        string `json:"label"`
	InitialValue int64  `json:"initial_value"`
	MaxDelta     int64  `json:"max_delta"`
}

// Validate validates the create counter request
func (r *CreateCounterRequest) Validate() error {
	// Sanitize and validate label if provided
	if r.Label != "" {
		sanitized, err := utils.SanitizeLabel(r.Label)
		if err != nil {
			return err
		}
		r.Label = sanitized
	}

	// Validate initial value
	if err := utils.ValidateCounterValue(r.InitialValue); err != nil {
		return err
	}

	// Default MaxDelta to 50 if not provided or zero
	if r.MaxDelta == 0 {
		r.MaxDelta = 50
	}

	// Validate max_delta is at least 1
	if r.MaxDelta < 1 {
		return errors.New("max_delta must be at least 1")
	}

	return nil
}

// IncrementResponse represents a response to an increment operation
type IncrementResponse struct {
	OperationID string    `json:"operation_id,omitempty"`
	CounterID   string    `json:"counter_id"`
	Delta       int64     `json:"delta,omitempty"`
	Value       int64     `json:"value"`
	Replayed    bool      `json:"replayed"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// SetValueResponse represents a response to a set value operation
type SetValueResponse struct {
	OperationID string    `json:"operation_id,omitempty"`
	CounterID   string    `json:"counter_id"`
	Delta       int64     `json:"delta,omitempty"`
	Value       int64     `json:"value"`
	Replayed    bool      `json:"replayed"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// SetCounterValueRequest represents a request to set a counter value
type SetCounterValueRequest struct {
	Value *int64 `json:"value"`
}

// ListCountersResponse represents the response for listing counters
type ListCountersResponse struct {
	Counters   []Counter `json:"counters"`
	NextCursor *string   `json:"next_cursor"`
}

// Validate validates the set counter value request
func (r *SetCounterValueRequest) Validate() error {
	if r.Value == nil {
		return errors.New("value is required")
	}

	// Validate the value is within safe bounds
	if err := utils.ValidateCounterValue(*r.Value); err != nil {
		return err
	}

	return nil
}
