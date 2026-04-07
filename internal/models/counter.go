package models

import (
	"errors"
	"time"
)

// Counter represents a counter in the system
type Counter struct {
	ID        string    `json:"counter_id" db:"id"`
	TenantID  string    `json:"tenant_id" db:"tenant_id"`
	Label     string    `json:"label" db:"label"`
	Value     int64     `json:"value" db:"value"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
	UpdatedAt time.Time `json:"updated_at" db:"updated_at"`
}

// Validate validates the counter data
func (c *Counter) Validate() error {
	if c == nil {
		return errors.New("counter is nil")
	}
	if c.TenantID == "" {
		return errors.New("tenant_id is required")
	}
	return nil
}

// CreateCounterRequest represents a request to create a counter
type CreateCounterRequest struct {
	Label        string `json:"label"`
	InitialValue int64  `json:"initial_value"`
}

// Validate validates the create counter request
func (r *CreateCounterRequest) Validate() error {
	// Label is optional
	return nil
}

// IncrementResponse represents a response to an increment operation
type IncrementResponse struct {
	CounterID string    `json:"counter_id"`
	Value     int64     `json:"value"`
	UpdatedAt time.Time `json:"updated_at"`
}

// SetValueResponse represents a response to a set value operation
type SetValueResponse struct {
	CounterID string    `json:"counter_id"`
	Value     int64     `json:"value"`
	UpdatedAt time.Time `json:"updated_at"`
}
