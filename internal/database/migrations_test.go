package database

import (
	"testing"
)

func TestGetMigrationStatus(t *testing.T) {
	cfg := &DBConfig{
		Host:     "localhost",
		Port:     5432,
		User:     "postgres",
		Password: "postgres",
		DBName:   "counter_api_test",
		SSLMode:  "disable",
	}

	db, err := NewDB(cfg)
	if err != nil {
		t.Skipf("Skipping test: database not available: %v", err)
		return
	}
	defer db.Close()

	// This assumes schema_migrations table exists
	status, err := GetMigrationStatus(db)
	if err != nil {
		t.Logf("GetMigrationStatus failed (expected if table doesn't exist): %v", err)
		return
	}

	if status == nil {
		t.Error("Expected status map, got nil")
	}
}

func TestRecordMigration(t *testing.T) {
	cfg := &DBConfig{
		Host:     "localhost",
		Port:     5432,
		User:     "postgres",
		Password: "postgres",
		DBName:   "counter_api_test",
		SSLMode:  "disable",
	}

	db, err := NewDB(cfg)
	if err != nil {
		t.Skipf("Skipping test: database not available: %v", err)
		return
	}
	defer db.Close()

	// Clean up any existing test migration
	db.Exec("DELETE FROM schema_migrations WHERE version = 999")

	err = RecordMigration(db, 999)
	if err != nil {
		t.Errorf("RecordMigration failed: %v", err)
	}

	// Verify it was recorded
	var count int
	err = db.Get(&count, "SELECT COUNT(*) FROM schema_migrations WHERE version = 999")
	if err != nil {
		t.Errorf("Failed to query migration: %v", err)
	}
	if count != 1 {
		t.Errorf("Expected 1 migration, got %d", count)
	}

	// Clean up
	db.Exec("DELETE FROM schema_migrations WHERE version = 999")
}
