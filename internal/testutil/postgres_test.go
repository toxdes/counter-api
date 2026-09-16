package testutil

import (
	"counter/internal/migrations"
	"sync"
	"testing"
)

func TestOpenPostgresUsesRealMigrations(t *testing.T) {
	db := OpenPostgres(t)

	var migrationCount int
	if err := db.Get(&migrationCount, "SELECT COUNT(*) FROM schema_migrations"); err != nil {
		t.Fatalf("failed to query migration history: %v", err)
	}
	if migrationCount != 6 {
		t.Fatalf("expected 6 applied migrations, got %d", migrationCount)
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

	var hasPaginationIndex bool
	if err := db.Get(&hasPaginationIndex, `
		SELECT EXISTS (
			SELECT 1
			FROM pg_indexes
			WHERE schemaname = current_schema()
			  AND tablename = 'counters'
			  AND indexname = 'idx_counters_tenant_created_id'
		)
	`); err != nil {
		t.Fatalf("failed to verify counter pagination index: %v", err)
	}
	if !hasPaginationIndex {
		t.Fatal("real migrations did not add the counter pagination index")
	}

	var hasOperationHistory bool
	if err := db.Get(&hasOperationHistory, `
		SELECT EXISTS (
			SELECT 1
			FROM information_schema.tables
			WHERE table_schema = current_schema()
			  AND table_name = 'counter_operations'
		)
	`); err != nil {
		t.Fatalf("failed to verify operation history table: %v", err)
	}
	if !hasOperationHistory {
		t.Fatal("real migrations did not add the operation history table")
	}

	if _, err := db.Exec(`
		INSERT INTO tenants (label) VALUES ('fixture-tenant');
	`); err != nil {
		t.Fatalf("fixture schema is not usable: %v", err)
	}
}

func TestConcurrentMigrationRunsAreSerialized(t *testing.T) {
	db := OpenPostgres(t)
	if _, err := db.Exec("DROP INDEX idx_counters_tenant_created_id"); err != nil {
		t.Fatalf("failed to prepare pending migration: %v", err)
	}
	if _, err := db.Exec("DELETE FROM schema_migrations WHERE version = 4"); err != nil {
		t.Fatalf("failed to prepare migration tracking: %v", err)
	}

	start := make(chan struct{})
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			errs <- migrations.RunUp(db)
		}()
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent migration run failed: %v", err)
		}
	}

	var migrationCount int
	if err := db.Get(&migrationCount, "SELECT COUNT(*) FROM schema_migrations"); err != nil {
		t.Fatalf("failed to query migration history: %v", err)
	}
	if migrationCount != 6 {
		t.Fatalf("expected six applied migrations after concurrent runs, got %d", migrationCount)
	}
}

func TestMigrationDownRollsBackOneVersion(t *testing.T) {
	db := OpenPostgres(t)
	if err := migrations.RunDown(db); err != nil {
		t.Fatalf("roll back latest migration: %v", err)
	}

	var migrationCount int
	if err := db.Get(&migrationCount, "SELECT COUNT(*) FROM schema_migrations"); err != nil {
		t.Fatalf("failed to query migration history: %v", err)
	}
	if migrationCount != 5 {
		t.Fatalf("expected one migration to be rolled back, got %d remaining", migrationCount)
	}

	if err := migrations.RunUp(db); err != nil {
		t.Fatalf("reapply rolled-back migration: %v", err)
	}
}
