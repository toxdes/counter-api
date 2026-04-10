package models

import (
	"testing"
	"time"
)

func TestCounterValidate(t *testing.T) {
	tests := []struct {
		name    string
		counter *Counter
		wantErr bool
	}{
		{
			name: "valid counter with tenant",
			counter: &Counter{
				TenantID: "123e4567-e89b-12d3-a456-426614174000",
				Value:    0,
			},
			wantErr: false,
		},
		{
			name: "valid counter with label",
			counter: &Counter{
				TenantID: "123e4567-e89b-12d3-a456-426614174000",
				Label:    "likes",
				Value:    0,
			},
			wantErr: false,
		},
		{
			name: "missing tenant_id",
			counter: &Counter{
				TenantID: "",
				Value:    0,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.counter.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestCreateCounterRequestValidate(t *testing.T) {
	tests := []struct {
		name    string
		req     *CreateCounterRequest
		wantErr bool
	}{
		{
			name: "valid request",
			req: &CreateCounterRequest{
				Label:        "likes",
				InitialValue: 0,
			},
			wantErr: false,
		},
		{
			name: "missing tenant_id",
			req: &CreateCounterRequest{
				InitialValue: 0,
			},
			wantErr: false, // label is optional
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.req.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestIncrementResponse(t *testing.T) {
	now := time.Now().UTC()
	resp := &IncrementResponse{
		CounterID: "123e4567-e89b-12d3-a456-426614174000",
		Value:     42,
		UpdatedAt: now,
	}

	if resp.CounterID == "" {
		t.Error("Expected CounterID to be set")
	}
	if resp.Value != 42 {
		t.Errorf("Expected value 42, got %d", resp.Value)
	}
}

func TestCounterWithMaxDelta(t *testing.T) {
	counter := &Counter{
		TenantID: "123e4567-e89b-12d3-a456-426614174000",
		Label:    "likes",
		Value:    0,
		MaxDelta: 100,
	}

	err := counter.Validate()
	if err != nil {
		t.Errorf("Validate() error = %v", err)
	}

	if counter.MaxDelta != 100 {
		t.Errorf("Expected MaxDelta 100, got %d", counter.MaxDelta)
	}
}

func TestCreateCounterRequestMaxDeltaValidation(t *testing.T) {
	tests := []struct {
		name    string
		req     *CreateCounterRequest
		wantErr bool
	}{
		{
			name: "valid max_delta",
			req: &CreateCounterRequest{
				Label:        "likes",
				InitialValue: 0,
				MaxDelta:     100,
			},
			wantErr: false,
		},
		{
			name: "max_delta defaults to 50 when zero",
			req: &CreateCounterRequest{
				Label:        "likes",
				InitialValue: 0,
				MaxDelta:     0,
			},
			wantErr: false,
		},
		{
			name: "max_delta less than 1 should fail",
			req: &CreateCounterRequest{
				Label:        "likes",
				InitialValue: 0,
				MaxDelta:     -5,
			},
			wantErr: true,
		},
		{
			name: "max_delta of 1 is valid",
			req: &CreateCounterRequest{
				Label:        "likes",
				InitialValue: 0,
				MaxDelta:     1,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.req.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
			// Check that MaxDelta is defaulted to 50 if 0
			if tt.req.MaxDelta == 0 && tt.req.MaxDelta != 50 {
				t.Errorf("Expected MaxDelta to be defaulted to 50, got %d", tt.req.MaxDelta)
			}
		})
	}
}
