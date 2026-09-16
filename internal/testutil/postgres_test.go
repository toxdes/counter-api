package testutil

import "testing"

func TestOpenPostgresUsesRealMigrations(t *testing.T) {
	db := OpenPostgres(t)

	var migrationCount int
	if err := db.Get(&migrationCount, "SELECT COUNT(*) FROM schema_migrations"); err != nil {
		t.Fatalf("failed to query migration history: %v", err)
	}
	if migrationCount != 3 {
		t.Fatalf("expected 3 applied migrations, got %d", migrationCount)
	}

	var hasMaxDelta bool
	if err := db.Get(&hasMaxDelta, `
		SELECT EXISTS (
			SELECT 1
			FROM information_schema.columns
			WHERE table_schema = current_schema()
			  AND table_name = 'counters'
			  AND column_name = 'max_delta'
		)
	`); err != nil {
		t.Fatalf("failed to verify migrated counter schema: %v", err)
	}
	if !hasMaxDelta {
		t.Fatal("real migrations did not add counters.max_delta")
	}

	if _, err := db.Exec(`
		INSERT INTO tenants (label) VALUES ('fixture-tenant');
	`); err != nil {
		t.Fatalf("fixture schema is not usable: %v", err)
	}
}
