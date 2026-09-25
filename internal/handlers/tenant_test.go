package handlers

import (
	"counter/internal/database"
	"counter/internal/models"
	"counter/internal/testutil"
	"encoding/json"
	"testing"

	"github.com/valyala/fasthttp"
)

func TestCreateTenantHandler(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	defer cleanupTestDB(t, db)

	handler := CreateTenantHandler(db)

	body := `{"label": "blog"}`
	ctx := &fasthttp.RequestCtx{}
	ctx.Request.SetBody([]byte(body))
	ctx.Request.Header.SetMethod("POST")
	ctx.Request.SetRequestURI("/tenants")
	ctx.Request.Header.SetContentType("application/json")

	handler(ctx)

	if ctx.Response.StatusCode() != fasthttp.StatusCreated {
		t.Errorf("Expected status 201, got %d: %s", ctx.Response.StatusCode(), ctx.Response.Body())
	}

	var resp models.Tenant
	if err := json.Unmarshal(ctx.Response.Body(), &resp); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if resp.Label != "blog" {
		t.Errorf("Expected label 'blog', got '%s'", resp.Label)
	}
	if resp.ID == "" {
		t.Error("Expected tenant ID to be set")
	}
}

func TestGetTenantHandler(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	defer cleanupTestDB(t, db)

	// Create a tenant first
	tenantID := createTestTenant(t, db, "blog")

	handler := GetTenantHandler(db)

	ctx := &fasthttp.RequestCtx{}
	ctx.Request.SetRequestURI("/tenants/" + tenantID)
	ctx.Request.Header.SetMethod("GET")

	handler(ctx)

	if ctx.Response.StatusCode() != fasthttp.StatusOK {
		t.Errorf("Expected status 200, got %d: %s", ctx.Response.StatusCode(), ctx.Response.Body())
	}

	var resp models.Tenant
	if err := json.Unmarshal(ctx.Response.Body(), &resp); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if resp.ID != tenantID {
		t.Errorf("Expected ID %s, got %s", tenantID, resp.ID)
	}
}

func TestGetTenantNotFound(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	defer cleanupTestDB(t, db)

	handler := GetTenantHandler(db)

	ctx := &fasthttp.RequestCtx{}
	ctx.Request.SetRequestURI("/tenants/00000000-0000-0000-0000-000000000000")
	ctx.Request.Header.SetMethod("GET")

	handler(ctx)

	if ctx.Response.StatusCode() != fasthttp.StatusNotFound {
		t.Errorf("Expected status 404, got %d", ctx.Response.StatusCode())
	}
}

// Helper functions
func setupTestDB(t *testing.T) *database.DB {
	return testutil.OpenPostgres(t)
}

func cleanupTestDB(t *testing.T, db *database.DB) {
	// The shared fixture owns an isolated schema and cleans it up through t.Cleanup.
}

func createTestTenant(t *testing.T, db *database.DB, label string) string {
	var id string
	err := db.QueryRow("INSERT INTO tenants (label) VALUES ($1) RETURNING id", label).Scan(&id)
	if err != nil {
		t.Fatalf("Failed to create test tenant: %v", err)
	}
	return id
}
