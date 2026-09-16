package migrations

import (
	"strings"
	"testing"
)

func TestEmbeddedMigrationHistoryIsCanonicalAndPaired(t *testing.T) {
	files, err := discoverMigrations()
	if err != nil {
		t.Fatalf("discover migrations: %v", err)
	}
	if len(files) != 6 {
		t.Fatalf("expected six canonical migrations, got %d", len(files))
	}

	for index, file := range files {
		if file.version != int64(index+1) {
			t.Fatalf("migration %d has version %d", index+1, file.version)
		}
		if file.up == "" || file.down == "" {
			t.Fatalf("migration %d is missing an up/down pair: %#v", file.version, file)
		}
	}

	if !strings.Contains(files[2].up, "ADD COLUMN max_delta") {
		t.Fatal("version 3 no longer represents the max_delta migration")
	}
	if !strings.Contains(files[3].up, "tenant_id, created_at, id") {
		t.Fatal("version 4 does not define the counter pagination index")
	}
	if !strings.Contains(files[4].up, "opening_balance") || !strings.Contains(files[5].up, "initial_value") {
		t.Fatal("operation kind history does not migrate to initial_value")
	}
}

func TestValidateMigrationPathsRejectsUnpairedVersions(t *testing.T) {
	err := validateMigrationPaths(
		[]string{"000001_init.up.sql", "000002_labels.up.sql"},
		[]string{"000001_init.down.sql"},
	)
	if err == nil || !strings.Contains(err.Error(), "missing down migration for version 2") {
		t.Fatalf("expected missing-pair error, got %v", err)
	}
}

func TestValidateMigrationPathsRejectsDuplicateVersions(t *testing.T) {
	err := validateMigrationPaths(
		[]string{"000001_init.up.sql", "000001_again.up.sql"},
		[]string{"000001_init.down.sql"},
	)
	if err == nil || !strings.Contains(err.Error(), "duplicate up migration version 1") {
		t.Fatalf("expected duplicate-version error, got %v", err)
	}
}
