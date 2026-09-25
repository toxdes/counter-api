package main

import "testing"

func TestDatabaseURLForCommandUsesMigrationURLOnlyForMigration(t *testing.T) {
	runtimeURL := "postgres://pooled/db"
	migrationURL := "postgres://direct/db"

	if got := databaseURLForCommand(runtimeURL, migrationURL, false); got != runtimeURL {
		t.Fatalf("runtime command URL = %q, want runtime URL %q", got, runtimeURL)
	}
	if got := databaseURLForCommand(runtimeURL, migrationURL, true); got != migrationURL {
		t.Fatalf("migration command URL = %q, want migration URL %q", got, migrationURL)
	}
	if got := databaseURLForCommand(runtimeURL, "", true); got != runtimeURL {
		t.Fatalf("migration command without override = %q, want runtime URL %q", got, runtimeURL)
	}
}
