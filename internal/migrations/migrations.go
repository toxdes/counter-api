package migrations

import (
	"counter/internal/database"
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strings"
)

//go:embed *.sql
var migrationFiles embed.FS

// RunUp executes all pending up migrations
func RunUp(db *database.DB) error {
	// Get current migration status
	status, err := database.GetMigrationStatus(db)
	if err != nil {
		return fmt.Errorf("failed to get migration status: %w", err)
	}

	// Create schema_migrations table if it doesn't exist
	if err := ensureSchemaMigrationsTable(db); err != nil {
		return fmt.Errorf("failed to create schema_migrations table: %w", err)
	}

	// Get all up migration files
	files, err := getUpMigrationFiles()
	if err != nil {
		return fmt.Errorf("failed to get migration files: %w", err)
	}

	// Run migrations in order
	for _, file := range files {
		version, err := extractVersion(file)
		if err != nil {
			return fmt.Errorf("failed to extract version from %s: %w", file, err)
		}

		// Skip if already applied
		if status[version] {
			fmt.Printf("Migration %d already applied, skipping\n", version)
			continue
		}

		// Read migration file
		content, err := migrationFiles.ReadFile(file)
		if err != nil {
			return fmt.Errorf("failed to read migration file %s: %w", file, err)
		}

		// Execute migration
		fmt.Printf("Applying migration %d from %s\n", version, file)
		if _, err := db.Exec(string(content)); err != nil {
			return fmt.Errorf("failed to apply migration %d: %w", version, err)
		}

		// Record migration (if it doesn't already record itself)
		if !isSchemaMigrationFile(file) {
			if err := database.RecordMigration(db, version); err != nil {
				return fmt.Errorf("failed to record migration %d: %w", version, err)
			}
		}

		fmt.Printf("Migration %d applied successfully\n", version)
	}

	return nil
}

// RunDown executes all down migrations (for rollback)
func RunDown(db *database.DB) error {
	// Get current migration status
	status, err := database.GetMigrationStatus(db)
	if err != nil {
		return fmt.Errorf("failed to get migration status: %w", err)
	}

	// Get all down migration files
	files, err := getDownMigrationFiles()
	if err != nil {
		return fmt.Errorf("failed to get migration files: %w", err)
	}

	// Run migrations in reverse order
	for i := len(files) - 1; i >= 0; i-- {
		file := files[i]
		version, err := extractVersion(file)
		if err != nil {
			return fmt.Errorf("failed to extract version from %s: %w", file, err)
		}

		// Skip if not applied
		if !status[version] {
			fmt.Printf("Migration %d not applied, skipping\n", version)
			continue
		}

		// Read migration file
		content, err := migrationFiles.ReadFile(file)
		if err != nil {
			return fmt.Errorf("failed to read migration file %s: %w", file, err)
		}

		// Execute migration
		fmt.Printf("Rolling back migration %d from %s\n", version, file)
		if _, err := db.Exec(string(content)); err != nil {
			return fmt.Errorf("failed to rollback migration %d: %w", version, err)
		}

		fmt.Printf("Migration %d rolled back successfully\n", version)
	}

	return nil
}

// ensureSchemaMigrationsTable creates the schema_migrations table if it doesn't exist
func ensureSchemaMigrationsTable(db *database.DB) error {
	query := `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version BIGINT PRIMARY KEY,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);
	`
	_, err := db.Exec(query)
	return err
}

// getUpMigrationFiles returns all .up.sql files sorted by version
func getUpMigrationFiles() ([]string, error) {
	var files []string
	err := fs.WalkDir(migrationFiles, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(path, ".up.sql") {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(files)
	return files, nil
}

// getDownMigrationFiles returns all .down.sql files sorted by version
func getDownMigrationFiles() ([]string, error) {
	var files []string
	err := fs.WalkDir(migrationFiles, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(path, ".down.sql") {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(files)
	return files, nil
}

// extractVersion extracts the version number from a migration filename
// Expected format: XXX_name.up.sql or XXX_name.down.sql
func extractVersion(filename string) (int64, error) {
	// Remove directory path
	parts := strings.Split(filename, "/")
	name := parts[len(parts)-1]

	// Extract version number (first number before underscore)
	var version int64
	_, err := fmt.Sscanf(name, "%d_", &version)
	if err != nil {
		return 0, fmt.Errorf("invalid migration filename format: %s", filename)
	}
	return version, nil
}

// isSchemaMigrationFile checks if the file is the schema_migrations.sql file
func isSchemaMigrationFile(filename string) bool {
	return strings.HasSuffix(filename, "schema_migrations.sql")
}
