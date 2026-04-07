package handlers

import (
	"encoding/json"
	"testing"
)

func TestErrorResponse(t *testing.T) {
	resp := NewErrorResponse("TENANT_NOT_FOUND", "Tenant not found")

	if len(resp.Errors) != 1 {
		t.Errorf("Expected 1 error, got %d", len(resp.Errors))
	}

	if resp.Errors[0].Code != "TENANT_NOT_FOUND" {
		t.Errorf("Expected code 'TENANT_NOT_FOUND', got '%s'", resp.Errors[0].Code)
	}

	if resp.Errors[0].Message != "Tenant not found" {
		t.Errorf("Expected message 'Tenant not found', got '%s'", resp.Errors[0].Message)
	}
}

func TestErrorResponseJSON(t *testing.T) {
	resp := NewErrorResponse("RATE_LIMIT_EXCEEDED", "Too many requests")

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("Failed to marshal error response: %v", err)
	}

	var parsed map[string][]map[string]string
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("Failed to unmarshal error response: %v", err)
	}

	if len(parsed["errors"]) != 1 {
		t.Errorf("Expected 1 error in JSON, got %d", len(parsed["errors"]))
	}
}
