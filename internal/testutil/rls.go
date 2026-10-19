package testutil

import (
	"counter/internal/database"
	urlpkg "net/url"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
)

// OpenRestrictedPostgres provisions a real non-owner LOGIN role. Superuser
// fixture connections are used only for setup/assertions, never API requests.
func OpenRestrictedPostgres(t *testing.T) (*database.DB, *database.DB) {
	t.Helper()
	owner := OpenPostgres(t)
	var schema string
	if err := owner.Get(&schema, "SELECT current_schema()"); err != nil {
		t.Fatal(err)
	}
	role := "counter_runtime_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	password := uuid.NewString()
	quotedRole := quoteIdentifier(role)
	if _, err := owner.Exec("CREATE ROLE " + quotedRole + " LOGIN NOSUPERUSER NOBYPASSRLS NOCREATEDB NOCREATEROLE NOINHERIT PASSWORD '" + password + "'"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := owner.Exec("DROP OWNED BY " + quotedRole); err != nil {
			t.Error(err)
		}
		if _, err := owner.Exec("DROP ROLE " + quotedRole); err != nil {
			t.Error(err)
		}
	})
	if _, err := owner.Exec("GRANT USAGE ON SCHEMA " + quoteIdentifier(schema) + " TO " + quotedRole); err != nil {
		t.Fatal(err)
	}
	if _, err := owner.Exec("GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA " + quoteIdentifier(schema) + " TO " + quotedRole); err != nil {
		t.Fatal(err)
	}
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		url = os.Getenv("DATABASE_URL")
	}
	if url == "" {
		url = "postgres://postgres:postgres@localhost:5432/counter_api_test?sslmode=disable"
	}
	scoped, err := withSearchPath(url, schema)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := urlpkg.Parse(scoped)
	if err != nil {
		t.Fatal(err)
	}
	parsed.User = urlpkg.UserPassword(role, password)
	runtime, err := database.NewDB(&database.DBConfig{DatabaseURL: parsed.String(), MaxOpenConns: 8, MaxIdleConns: 2})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := runtime.Close(); err != nil {
			t.Error(err)
		}
	})
	return owner, runtime
}
