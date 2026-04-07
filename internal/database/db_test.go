package database

import (
	"testing"
)

func TestNewDB(t *testing.T) {
	cfg := &DBConfig{
		Host:     "localhost",
		Port:     5432,
		User:     "postgres",
		Password: "postgres",
		DBName:   "counter_api_test",
		SSLMode:  "disable",
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
		Host:         "localhost",
		Port:         5432,
		User:         "postgres",
		Password:     "postgres",
		DBName:       "counter_api_test",
		SSLMode:      "disable",
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
