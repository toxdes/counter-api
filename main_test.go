package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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

func TestEnvironmentFileLoadsWithoutOverridingProcessEnvironment(t *testing.T) {
	const supplied = "COUNTER_ENV_FILE_TEST_SUPPLIED"
	const preserved = "COUNTER_ENV_FILE_TEST_PRESERVED"
	// Setenv registers cleanup, then Unsetenv models a genuinely missing key.
	t.Setenv(supplied, "")
	if err := os.Unsetenv(supplied); err != nil {
		t.Fatal(err)
	}
	t.Setenv(preserved, "process-value")
	path := filepath.Join(t.TempDir(), "custom file.env")
	if err := os.WriteFile(path, []byte(supplied+"='file value'\n"+preserved+"=file-value\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := loadEnvironmentFile(path, true); err != nil {
		t.Fatal(err)
	}
	if got := os.Getenv(supplied); got != "file value" {
		t.Fatalf("file value = %q", got)
	}
	if got := os.Getenv(preserved); got != "process-value" {
		t.Fatalf("existing process value changed to %q", got)
	}
	// An intentionally empty process variable also takes precedence.
	t.Setenv(preserved, "")
	if err := loadEnvironmentFile(path, true); err != nil {
		t.Fatal(err)
	}
	if os.Getenv(preserved) != "" {
		t.Fatal("empty process variable was overwritten")
	}
}

func TestEnvironmentFileErrorsDoNotExposeContents(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.env")
	if err := loadEnvironmentFile(path, false); err != nil {
		t.Fatalf("optional default must remain compatible: %v", err)
	}
	if err := loadEnvironmentFile(path, true); err == nil {
		t.Fatal("explicit missing file must fail")
	}
	const secret = "fixture-secret-must-not-appear"
	if err := os.WriteFile(path, []byte("COUNTER_ENV_FILE_BAD='"+secret+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	err := loadEnvironmentFile(path, true)
	if err == nil || !strings.Contains(err.Error(), "invalid dotenv syntax") {
		t.Fatalf("expected invalid dotenv error, got %v", err)
	}
	if strings.Contains(err.Error(), secret) || strings.Contains(err.Error(), "COUNTER_ENV_FILE_BAD") {
		t.Fatal("parser error exposed env file contents")
	}
}
