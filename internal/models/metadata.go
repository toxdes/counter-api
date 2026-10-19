package models

import (
	"encoding/json"
	"time"
)

// CounterMetadata is one independent, counter-owned JSON document. Its revision
// is opaque and changes on every replacement, including after delete/recreate.
type CounterMetadata struct {
	TenantID  string          `json:"tenant_id" db:"tenant_id"`
	CounterID string          `json:"counter_id" db:"counter_id"`
	Data      json.RawMessage `json:"data" db:"data"`
	Version   string          `json:"version" db:"version"`
	CreatedAt time.Time       `json:"created_at" db:"created_at"`
	UpdatedAt time.Time       `json:"updated_at" db:"updated_at"`
}
