// Package testutil contains shared integration-test fixtures.
package testutil

import (
	"counter/internal/database"
	"counter/internal/migrations"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/joho/godotenv"
)

// OpenPostgres creates an isolated PostgreSQL schema and applies the real
// embedded migrations to it. Tests use a per-connection search_path option so
// every connection in the pool targets the isolated schema.
func OpenPostgres(t *testing.T) *database.DB {
	t.Helper()
	_ = godotenv.Load()

	dbURL := os.Getenv("TEST_DATABASE_URL")
	if dbURL == "" {
		dbURL = os.Getenv("DATABASE_URL")
	}
	if dbURL == "" {
		dbURL = "postgres://postgres:postgres@localhost:5432/counter_api_test?sslmode=disable"
	}

	adminDB, err := database.NewDB(&database.DBConfig{
		DatabaseURL:  dbURL,
		MaxOpenConns: 1,
		MaxIdleConns: 1,
	})
	if err != nil {
		handleUnavailable(t, err)
		return nil
	}

	schema := "counter_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	quotedSchema := quoteIdentifier(schema)
	if _, err := adminDB.Exec("CREATE SCHEMA " + quotedSchema); err != nil {
		adminDB.Close()
		t.Fatalf("failed to create test schema: %v", err)
	}

	schemaURL, err := withSearchPath(dbURL, schema)
	if err != nil {
		adminDB.Close()
		t.Fatalf("failed to configure test schema search path: %v", err)
	}

	db, err := database.NewDB(&database.DBConfig{
		DatabaseURL:  schemaURL,
		MaxOpenConns: 10,
		MaxIdleConns: 10,
	})
	if err != nil {
		_, _ = adminDB.Exec("DROP SCHEMA " + quotedSchema + " CASCADE")
		adminDB.Close()
		handleUnavailable(t, err)
		return nil
	}

	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("failed to close test database: %v", err)
		}
		if _, err := adminDB.Exec("DROP SCHEMA " + quotedSchema + " CASCADE"); err != nil {
			t.Errorf("failed to drop test schema: %v", err)
		}
		if err := adminDB.Close(); err != nil {
			t.Errorf("failed to close admin database: %v", err)
		}
	})

	if err := migrations.RunUp(db); err != nil {
		t.Fatalf("failed to apply test migrations: %v", err)
	}
	return db
}

func handleUnavailable(t *testing.T, err error) {
	t.Helper()
	if os.Getenv("COUNTER_REQUIRE_POSTGRES") == "1" {
		t.Fatalf("PostgreSQL is required for integration tests: %v", err)
	}
	t.Skipf("PostgreSQL unavailable: %v", err)
}

func withSearchPath(rawURL, schema string) (string, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}
	query := parsed.Query()
	query.Set("options", "-csearch_path="+schema+",public")
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}

func quoteIdentifier(identifier string) string {
	return fmt.Sprintf(`"%s"`, strings.ReplaceAll(identifier, `"`, `""`))
}
