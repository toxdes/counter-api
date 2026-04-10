package handlers

import (
	"counter/internal/database"
	"counter/internal/models"
	"encoding/json"
	"testing"

	"github.com/valyala/fasthttp"
)

func TestCreateCounterHandler(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	defer cleanupTestDB(t, db)

	tenantID := createTestTenant(t, db, "blog")
	handler := CreateCounterHandler(db)

	body := `{"label": "likes", "initial_value": 0}`
	ctx := &fasthttp.RequestCtx{}
	ctx.Request.SetBody([]byte(body))
	ctx.Request.Header.SetMethod("POST")
	ctx.Request.SetRequestURI("/tenants/" + tenantID)
	ctx.Request.Header.SetContentType("application/json")

	// Set tenant_id in context (simulating router)
	ctx.SetUserValue("tenant_id", tenantID)

	handler(ctx)

	if ctx.Response.StatusCode() != fasthttp.StatusCreated {
		t.Errorf("Expected status 201, got %d: %s", ctx.Response.StatusCode(), ctx.Response.Body())
	}

	var resp models.Counter
	if err := json.Unmarshal(ctx.Response.Body(), &resp); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if resp.Label != "likes" {
		t.Errorf("Expected label 'likes', got '%s'", resp.Label)
	}
}

func TestIncrementCounterHandler(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	defer cleanupTestDB(t, db)

	tenantID := createTestTenant(t, db, "blog")
	counterID := createTestCounter(t, db, tenantID, "likes", 0)

	handler := IncrementCounterHandler(db)

	ctx := &fasthttp.RequestCtx{}
	ctx.Request.SetRequestURI("/tenants/" + tenantID + "/counters/" + counterID + "/inc?delta=5")
	ctx.Request.Header.SetMethod("POST")
	ctx.SetUserValue("tenant_id", tenantID)
	ctx.SetUserValue("counter_id", counterID)

	handler(ctx)

	if ctx.Response.StatusCode() != fasthttp.StatusOK {
		t.Errorf("Expected status 200, got %d: %s", ctx.Response.StatusCode(), ctx.Response.Body())
	}

	var resp models.IncrementResponse
	if err := json.Unmarshal(ctx.Response.Body(), &resp); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if resp.Value != 5 {
		t.Errorf("Expected value 5, got %d", resp.Value)
	}
}

func TestSetCounterValueHandler(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	defer cleanupTestDB(t, db)

	tenantID := createTestTenant(t, db, "blog")
	counterID := createTestCounter(t, db, tenantID, "likes", 0)

	handler := SetCounterValueHandler(db)

	ctx := &fasthttp.RequestCtx{}
	ctx.Request.SetRequestURI("/tenants/" + tenantID + "/counters/" + counterID + "/set?val=100")
	ctx.Request.Header.SetMethod("POST")
	ctx.SetUserValue("tenant_id", tenantID)
	ctx.SetUserValue("counter_id", counterID)

	handler(ctx)

	if ctx.Response.StatusCode() != fasthttp.StatusOK {
		t.Errorf("Expected status 200, got %d: %s", ctx.Response.StatusCode(), ctx.Response.Body())
	}

	var resp models.SetValueResponse
	if err := json.Unmarshal(ctx.Response.Body(), &resp); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if resp.Value != 100 {
		t.Errorf("Expected value 100, got %d", resp.Value)
	}
}

func TestGetCounterHandler(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	defer cleanupTestDB(t, db)

	tenantID := createTestTenant(t, db, "blog")
	counterID := createTestCounter(t, db, tenantID, "likes", 42)

	handler := GetCounterHandler(db)

	ctx := &fasthttp.RequestCtx{}
	ctx.Request.SetRequestURI("/tenants/" + tenantID + "/counters/" + counterID)
	ctx.Request.Header.SetMethod("GET")
	ctx.SetUserValue("tenant_id", tenantID)
	ctx.SetUserValue("counter_id", counterID)

	handler(ctx)

	if ctx.Response.StatusCode() != fasthttp.StatusOK {
		t.Errorf("Expected status 200, got %d: %s", ctx.Response.StatusCode(), ctx.Response.Body())
	}

	var resp models.Counter
	if err := json.Unmarshal(ctx.Response.Body(), &resp); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if resp.Value != 42 {
		t.Errorf("Expected value 42, got %d", resp.Value)
	}
}

func createTestCounter(t *testing.T, db *database.DB, tenantID, label string, value int64) string {
	var id string
	err := db.QueryRow(
		"INSERT INTO counters (tenant_id, label, value, max_delta) VALUES ($1, $2, $3, 50) RETURNING id",
		tenantID, label, value,
	).Scan(&id)
	if err != nil {
		t.Fatalf("Failed to create test counter: %v", err)
	}
	return id
}

func TestCreateCounterWithMaxDelta(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	defer cleanupTestDB(t, db)

	tenantID := createTestTenant(t, db, "blog")
	handler := CreateCounterHandler(db)

	// Test with explicit max_delta
	body := `{"label": "likes", "initial_value": 0, "max_delta": 100}`
	ctx := &fasthttp.RequestCtx{}
	ctx.Request.SetBody([]byte(body))
	ctx.Request.Header.SetMethod("POST")
	ctx.Request.SetRequestURI("/tenants/" + tenantID)
	ctx.Request.Header.SetContentType("application/json")
	ctx.SetUserValue("tenant_id", tenantID)

	handler(ctx)

	if ctx.Response.StatusCode() != fasthttp.StatusCreated {
		t.Errorf("Expected status 201, got %d: %s", ctx.Response.StatusCode(), ctx.Response.Body())
	}

	var resp models.Counter
	if err := json.Unmarshal(ctx.Response.Body(), &resp); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if resp.MaxDelta != 100 {
		t.Errorf("Expected MaxDelta 100, got %d", resp.MaxDelta)
	}

	// Test without max_delta (should default to 50)
	body2 := `{"label": "views", "initial_value": 0}`
	ctx2 := &fasthttp.RequestCtx{}
	ctx2.Request.SetBody([]byte(body2))
	ctx2.Request.Header.SetMethod("POST")
	ctx2.Request.SetRequestURI("/tenants/" + tenantID)
	ctx2.Request.Header.SetContentType("application/json")
	ctx2.SetUserValue("tenant_id", tenantID)

	handler(ctx2)

	if ctx2.Response.StatusCode() != fasthttp.StatusCreated {
		t.Errorf("Expected status 201, got %d: %s", ctx2.Response.StatusCode(), ctx2.Response.Body())
	}

	var resp2 models.Counter
	if err := json.Unmarshal(ctx2.Response.Body(), &resp2); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if resp2.MaxDelta != 50 {
		t.Errorf("Expected MaxDelta 50 (default), got %d", resp2.MaxDelta)
	}
}
