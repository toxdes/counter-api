package database

import (
	"fmt"
)

// GetMigrationStatus returns a map of applied migration versions
func GetMigrationStatus(db *DB) (map[int64]bool, error) {
	// Check if table exists
	var tableExists bool
	err := db.Get(&tableExists, `
		SELECT EXISTS (
			SELECT FROM information_schema.tables
			WHERE table_name = 'schema_migrations'
		)
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to check schema_migrations table: %w", err)
	}

	if !tableExists {
		return make(map[int64]bool), nil
	}

	rows, err := db.Query("SELECT version FROM schema_migrations")
	if err != nil {
		return nil, fmt.Errorf("failed to query migrations: %w", err)
	}
	defer rows.Close()

	status := make(map[int64]bool)
	for rows.Next() {
		var version int64
		if err := rows.Scan(&version); err != nil {
			return nil, fmt.Errorf("failed to scan migration version: %w", err)
		}
		status[version] = true
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating migrations: %w", err)
	}

	return status, nil
}

// RecordMigration records a migration as applied
func RecordMigration(db *DB, version int64) error {
	_, err := db.Exec(
		"INSERT INTO schema_migrations (version) VALUES ($1) ON CONFLICT (version) DO NOTHING",
		version,
	)
	if err != nil {
		return fmt.Errorf("failed to record migration: %w", err)
	}
	return nil
}
