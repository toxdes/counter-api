package router

import (
	"counter/internal/database"
	"counter/internal/middleware"
	"counter/internal/testutil"
	"testing"

	"github.com/valyala/fasthttp"
)

func TestRouterSetup(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	defer cleanupTestDB(t, db)

	corsConfig := &middleware.CORSConfig{
		AllowedOrigins:   "*",
		AllowedMethods:   "GET,POST,OPTIONS",
		AllowedHeaders:   "Content-Type,Authorization",
		AllowCredentials: false,
		MaxAge:           3600,
	}

	rateLimiter := middleware.NewRateLimiter(10, 1, 60)
	logger := middleware.NewDefaultLogger("info")

	router := NewRouter(db, corsConfig, rateLimiter, "test-key", logger, nil)

	if router == nil {
		t.Error("Expected router to be created")
	}
}

func TestAdminEndpointsRequireAuth(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	defer cleanupTestDB(t, db)

	corsConfig := &middleware.CORSConfig{
		AllowedOrigins:   "*",
		AllowedMethods:   "GET,POST,OPTIONS",
		AllowedHeaders:   "Content-Type,Authorization",
		AllowCredentials: false,
		MaxAge:           3600,
	}

	rateLimiter := middleware.NewRateLimiter(10, 1, 60)
	logger := middleware.NewDefaultLogger("info")

	router := NewRouter(db, corsConfig, rateLimiter, "test-key", logger, nil)

	// Test POST /tenants without auth
	ctx := &fasthttp.RequestCtx{}
	ctx.Request.SetRequestURI("/tenants")
	ctx.Request.Header.SetMethod("POST")
	ctx.Request.SetBody([]byte(`{"label": "blog"}`))
	ctx.Request.Header.SetContentType("application/json")

	router.ServeHTTP(ctx)

	if ctx.Response.StatusCode() != fasthttp.StatusUnauthorized {
		t.Errorf("Expected status 401 without auth, got %d", ctx.Response.StatusCode())
	}

	// Test GET /tenants/<id>/counters without auth
	ctx2 := &fasthttp.RequestCtx{}
	ctx2.Request.SetRequestURI("/tenants/00000000-0000-0000-0000-000000000000/counters")
	ctx2.Request.Header.SetMethod("GET")

	router.ServeHTTP(ctx2)

	if ctx2.Response.StatusCode() != fasthttp.StatusUnauthorized {
		t.Errorf("Expected status 401 for GET /tenants/<id>/counters without auth, got %d", ctx2.Response.StatusCode())
	}
}

func TestPublicEndpointsNoAuth(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	defer cleanupTestDB(t, db)

	corsConfig := &middleware.CORSConfig{
		AllowedOrigins:   "*",
		AllowedMethods:   "GET,POST,OPTIONS",
		AllowedHeaders:   "Content-Type,Authorization",
		AllowCredentials: false,
		MaxAge:           3600,
	}

	rateLimiter := middleware.NewRateLimiter(10, 1, 60)
	logger := middleware.NewDefaultLogger("info")

	router := NewRouter(db, corsConfig, rateLimiter, "test-key", logger, nil)

	// Create a tenant first
	tenantID := createTestTenant(t, db, "blog")
	_ = createTestCounter(t, db, tenantID, "likes", 0)

	// Test GET /tenants/{id} (no auth required)
	ctx := &fasthttp.RequestCtx{}
	ctx.Request.SetRequestURI("/tenants/" + tenantID)
	ctx.Request.Header.SetMethod("GET")

	router.ServeHTTP(ctx)

	// Should return 200 or 404 depending on if tenant exists
	if ctx.Response.StatusCode() != fasthttp.StatusOK && ctx.Response.StatusCode() != fasthttp.StatusNotFound {
		t.Errorf("Expected status 200 or 404, got %d", ctx.Response.StatusCode())
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

func createTestCounter(t *testing.T, db *database.DB, tenantID, label string, value int64) string {
	var id string
	err := db.QueryRow("INSERT INTO counters (tenant_id, label, value) VALUES ($1, $2, $3) RETURNING id", tenantID, label, value).Scan(&id)
	if err != nil {
		t.Fatalf("Failed to create test counter: %v", err)
	}
	return id
}
