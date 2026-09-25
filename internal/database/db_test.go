package database

import (
	"strings"
	"testing"
	"time"
)

func TestWithPostgresTimeoutsPreservesExistingOptions(t *testing.T) {
	value, err := withPostgresTimeouts(
		"postgres://localhost/counter?options=-c+search_path%3Dcounter",
		2*time.Second,
		500*time.Millisecond,
		10*time.Second,
	)
	if err != nil {
		t.Fatalf("withPostgresTimeouts() error = %v", err)
	}
	if value == "" || value == "postgres://localhost/counter?options=-c+search_path%3Dcounter" {
		t.Fatalf("database timeout options were not added: %s", value)
	}
	if !containsAll(value, []string{"statement_timeout", "lock_timeout", "idle_in_transaction_session_timeout", "search_path"}) {
		t.Fatalf("database URL lost expected options: %s", value)
	}
}

func TestWithPostgresTimeoutsSkipsNeonPoolerStartupOptions(t *testing.T) {
	url := "postgres://app:secret@ep-example-pooler.us-east-2.aws.neon.tech/counter?sslmode=require&channel_binding=require"
	got, err := withPostgresTimeouts(url, 2*time.Second, 500*time.Millisecond, 10*time.Second)
	if err != nil {
		t.Fatalf("withPostgresTimeouts() error = %v", err)
	}
	if got != url {
		t.Fatalf("Neon pooled URL changed: got %q, want unchanged URL %q", got, url)
	}
}

func TestIsNeonPoolerURL(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want bool
	}{
		{name: "Neon pooler", url: "postgres://user:pass@ep-example-pooler.us-east-2.aws.neon.tech/db", want: true},
		{name: "Neon direct", url: "postgres://user:pass@ep-example.us-east-2.aws.neon.tech/db", want: false},
		{name: "other postgres", url: "postgres://user:pass@db.example.com/db", want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := IsNeonPoolerURL(test.url); got != test.want {
				t.Fatalf("IsNeonPoolerURL() = %t, want %t", got, test.want)
			}
		})
	}
}

func containsAll(value string, parts []string) bool {
	for _, part := range parts {
		if !strings.Contains(value, part) {
			return false
		}
	}
	return true
}

func TestNewDB(t *testing.T) {
	cfg := &DBConfig{
		DatabaseURL: "postgres://postgres:postgres@localhost:5432/counter_api_test?sslmode=disable",
	}

	// This test requires a running PostgreSQL instance
	// In CI, this would use docker-compose or testcontainers
	db, err := NewDB(cfg)
	if err != nil {
		t.Skipf("Skipping test: database not available: %v", err)
		return
	}
	defer db.Close()

	if db.DB == nil {
		t.Error("Expected db.DB to be non-nil")
	}

	// Test connection
	err = db.Ping()
	if err != nil {
		t.Errorf("Failed to ping database: %v", err)
	}
}

func TestDBStats(t *testing.T) {
	cfg := &DBConfig{
		DatabaseURL:  "postgres://postgres:postgres@localhost:5432/counter_api_test?sslmode=disable",
		MaxOpenConns: 10,
		MaxIdleConns: 5,
	}

	db, err := NewDB(cfg)
	if err != nil {
		t.Skipf("Skipping test: database not available: %v", err)
		return
	}
	defer db.Close()

	stats := db.Stats()
	if stats.MaxOpenConnections != 10 {
		t.Errorf("Expected MaxOpenConnections 10, got %d", stats.MaxOpenConnections)
	}
}
