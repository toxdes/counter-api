package handlers

import (
	"counter/internal/database"
	"counter/internal/models"
	"encoding/base64"
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

func createTestCounterWithMaxDelta(t *testing.T, db *database.DB, tenantID, label string, value, maxDelta int64) string {
	var id string
	err := db.QueryRow(
		"INSERT INTO counters (tenant_id, label, value, max_delta) VALUES ($1, $2, $3, $4) RETURNING id",
		tenantID, label, value, maxDelta,
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

func TestIncrementCounterExceedsMaxDelta(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	defer cleanupTestDB(t, db)

	tenantID := createTestTenant(t, db, "blog")
	counterID := createTestCounterWithMaxDelta(t, db, tenantID, "likes", 0, 10)

	handler := IncrementCounterHandler(db)

	// Try to increment with delta=20 (exceeds max_delta=10)
	ctx := &fasthttp.RequestCtx{}
	ctx.Request.SetRequestURI("/tenants/" + tenantID + "/counters/" + counterID + "/inc?delta=20")
	ctx.Request.Header.SetMethod("POST")
	ctx.SetUserValue("tenant_id", tenantID)
	ctx.SetUserValue("counter_id", counterID)

	handler(ctx)

	if ctx.Response.StatusCode() != fasthttp.StatusBadRequest {
		t.Errorf("Expected status 400, got %d: %s", ctx.Response.StatusCode(), ctx.Response.Body())
	}

	// Verify error response
	var resp map[string][]map[string]string
	if err := json.Unmarshal(ctx.Response.Body(), &resp); err != nil {
		t.Fatalf("Failed to unmarshal error response: %v", err)
	}

	if len(resp["errors"]) != 1 {
		t.Errorf("Expected 1 error, got %d", len(resp["errors"]))
	}

	if resp["errors"][0]["code"] != "DELTA_EXCEEDS_MAXIMUM" {
		t.Errorf("Expected error code 'DELTA_EXCEEDS_MAXIMUM', got '%s'", resp["errors"][0]["code"])
	}
}

func TestIncrementCounterWithinMaxDelta(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	defer cleanupTestDB(t, db)

	tenantID := createTestTenant(t, db, "blog")
	counterID := createTestCounterWithMaxDelta(t, db, tenantID, "likes", 0, 10)

	handler := IncrementCounterHandler(db)

	// Increment with delta=5 (within max_delta=10)
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

func TestGetCounterIncludesMaxDelta(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	defer cleanupTestDB(t, db)

	tenantID := createTestTenant(t, db, "blog")
	counterID := createTestCounterWithMaxDelta(t, db, tenantID, "likes", 42, 100)

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

	if resp.MaxDelta != 100 {
		t.Errorf("Expected MaxDelta 100, got %d", resp.MaxDelta)
	}
}

func TestListCountersEmpty(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	defer cleanupTestDB(t, db)

	tenantID := createTestTenant(t, db, "blog")
	handler := ListCountersHandler(db)

	ctx := &fasthttp.RequestCtx{}
	ctx.Request.SetRequestURI("/tenants/" + tenantID + "/counters")
	ctx.Request.Header.SetMethod("GET")
	ctx.SetUserValue("tenant_id", tenantID)

	handler(ctx)

	if ctx.Response.StatusCode() != fasthttp.StatusOK {
		t.Errorf("Expected status 200, got %d: %s", ctx.Response.StatusCode(), ctx.Response.Body())
	}

	var resp models.ListCountersResponse
	if err := json.Unmarshal(ctx.Response.Body(), &resp); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if len(resp.Counters) != 0 {
		t.Errorf("Expected 0 counters, got %d", len(resp.Counters))
	}

	if resp.NextCursor != nil {
		t.Errorf("Expected nil next_cursor for empty list, got %v", *resp.NextCursor)
	}
}

func TestListCountersBasic(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	defer cleanupTestDB(t, db)

	tenantID := createTestTenant(t, db, "blog")
	c1 := createTestCounter(t, db, tenantID, "likes", 0)
	c2 := createTestCounter(t, db, tenantID, "views", 10)

	handler := ListCountersHandler(db)

	ctx := &fasthttp.RequestCtx{}
	ctx.Request.SetRequestURI("/tenants/" + tenantID + "/counters")
	ctx.Request.Header.SetMethod("GET")
	ctx.SetUserValue("tenant_id", tenantID)

	handler(ctx)

	if ctx.Response.StatusCode() != fasthttp.StatusOK {
		t.Errorf("Expected status 200, got %d: %s", ctx.Response.StatusCode(), ctx.Response.Body())
	}

	var resp models.ListCountersResponse
	if err := json.Unmarshal(ctx.Response.Body(), &resp); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if len(resp.Counters) != 2 {
		t.Errorf("Expected 2 counters, got %d", len(resp.Counters))
	}

	ids := map[string]bool{c1: false, c2: false}
	for _, c := range resp.Counters {
		ids[c.ID] = true
	}
	if !ids[c1] || !ids[c2] {
		t.Error("Not all expected counters returned")
	}

	if resp.NextCursor != nil {
		t.Errorf("Expected nil next_cursor, got %v", *resp.NextCursor)
	}
}

func TestListCountersPagination(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	defer cleanupTestDB(t, db)

	tenantID := createTestTenant(t, db, "blog")
	createTestCounter(t, db, tenantID, "a", 0)
	createTestCounter(t, db, tenantID, "b", 0)
	createTestCounter(t, db, tenantID, "c", 0)

	handler := ListCountersHandler(db)

	ctx := &fasthttp.RequestCtx{}
	ctx.Request.SetRequestURI("/tenants/" + tenantID + "/counters?limit=2")
	ctx.Request.Header.SetMethod("GET")
	ctx.SetUserValue("tenant_id", tenantID)

	handler(ctx)

	if ctx.Response.StatusCode() != fasthttp.StatusOK {
		t.Errorf("Expected status 200, got %d: %s", ctx.Response.StatusCode(), ctx.Response.Body())
	}

	var resp models.ListCountersResponse
	if err := json.Unmarshal(ctx.Response.Body(), &resp); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if len(resp.Counters) != 2 {
		t.Errorf("Expected 2 counters (limit=2), got %d", len(resp.Counters))
	}

	if resp.NextCursor == nil {
		t.Error("Expected non-nil next_cursor when more pages exist")
	}
}

func TestListCountersCursorContinuation(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	defer cleanupTestDB(t, db)

	tenantID := createTestTenant(t, db, "blog")
	c1 := createTestCounter(t, db, tenantID, "a", 0)
	c2 := createTestCounter(t, db, tenantID, "b", 0)
	c3 := createTestCounter(t, db, tenantID, "c", 0)

	handler := ListCountersHandler(db)

	// First page: limit=2
	ctx1 := &fasthttp.RequestCtx{}
	ctx1.Request.SetRequestURI("/tenants/" + tenantID + "/counters?limit=2")
	ctx1.Request.Header.SetMethod("GET")
	ctx1.SetUserValue("tenant_id", tenantID)

	handler(ctx1)

	if ctx1.Response.StatusCode() != fasthttp.StatusOK {
		t.Errorf("Expected status 200, got %d: %s", ctx1.Response.StatusCode(), ctx1.Response.Body())
	}

	var resp1 models.ListCountersResponse
	if err := json.Unmarshal(ctx1.Response.Body(), &resp1); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if resp1.NextCursor == nil {
		t.Fatal("Expected next_cursor on first page")
	}

	// Second page: use cursor
	ctx2 := &fasthttp.RequestCtx{}
	ctx2.Request.SetRequestURI("/tenants/" + tenantID + "/counters?limit=2&cursor=" + *resp1.NextCursor)
	ctx2.Request.Header.SetMethod("GET")
	ctx2.SetUserValue("tenant_id", tenantID)

	handler(ctx2)

	if ctx2.Response.StatusCode() != fasthttp.StatusOK {
		t.Errorf("Expected status 200, got %d: %s", ctx2.Response.StatusCode(), ctx2.Response.Body())
	}

	var resp2 models.ListCountersResponse
	if err := json.Unmarshal(ctx2.Response.Body(), &resp2); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if len(resp2.Counters) != 1 {
		t.Errorf("Expected 1 counter on last page, got %d", len(resp2.Counters))
	}

	if resp2.NextCursor != nil {
		t.Errorf("Expected nil next_cursor on last page, got %v", *resp2.NextCursor)
	}

	// Verify no overlap between pages
	page1IDs := map[string]bool{}
	for _, c := range resp1.Counters {
		page1IDs[c.ID] = true
	}
	for _, c := range resp2.Counters {
		if page1IDs[c.ID] {
			t.Errorf("Counter %s appears on both pages", c.ID)
		}
	}

	// Verify all 3 counters accounted for
	allIDs := map[string]bool{c1: false, c2: false, c3: false}
	for _, c := range resp1.Counters {
		allIDs[c.ID] = true
	}
	for _, c := range resp2.Counters {
		allIDs[c.ID] = true
	}
	for id, found := range allIDs {
		if !found {
			t.Errorf("Counter %s not found in any page", id)
		}
	}
}

func TestListCountersInvalidUUID(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	defer cleanupTestDB(t, db)

	handler := ListCountersHandler(db)

	ctx := &fasthttp.RequestCtx{}
	ctx.Request.SetRequestURI("/tenants/not-a-uuid/counters")
	ctx.Request.Header.SetMethod("GET")
	ctx.SetUserValue("tenant_id", "not-a-uuid")

	handler(ctx)

	if ctx.Response.StatusCode() != fasthttp.StatusBadRequest {
		t.Errorf("Expected status 400, got %d: %s", ctx.Response.StatusCode(), ctx.Response.Body())
	}
}

func TestListCountersTenantNotFound(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	defer cleanupTestDB(t, db)

	handler := ListCountersHandler(db)

	ctx := &fasthttp.RequestCtx{}
	ctx.Request.SetRequestURI("/tenants/00000000-0000-0000-0000-000000000000/counters")
	ctx.Request.Header.SetMethod("GET")
	ctx.SetUserValue("tenant_id", "00000000-0000-0000-0000-000000000000")

	handler(ctx)

	if ctx.Response.StatusCode() != fasthttp.StatusNotFound {
		t.Errorf("Expected status 404, got %d: %s", ctx.Response.StatusCode(), ctx.Response.Body())
	}
}

func TestListCountersSinglePage(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	defer cleanupTestDB(t, db)

	tenantID := createTestTenant(t, db, "blog")
	createTestCounter(t, db, tenantID, "likes", 0)
	createTestCounter(t, db, tenantID, "views", 10)
	createTestCounter(t, db, tenantID, "shares", 20)

	handler := ListCountersHandler(db)

	ctx := &fasthttp.RequestCtx{}
	ctx.Request.SetRequestURI("/tenants/" + tenantID + "/counters?limit=10")
	ctx.Request.Header.SetMethod("GET")
	ctx.SetUserValue("tenant_id", tenantID)

	handler(ctx)

	if ctx.Response.StatusCode() != fasthttp.StatusOK {
		t.Errorf("Expected status 200, got %d: %s", ctx.Response.StatusCode(), ctx.Response.Body())
	}

	var resp models.ListCountersResponse
	if err := json.Unmarshal(ctx.Response.Body(), &resp); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if len(resp.Counters) != 3 {
		t.Errorf("Expected 3 counters, got %d", len(resp.Counters))
	}

	if resp.NextCursor != nil {
		t.Errorf("Expected nil next_cursor, got %v", *resp.NextCursor)
	}
}

func decodeRawCursor(t *testing.T, cursor string) map[string]string {
	t.Helper()
	data, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		t.Fatalf("Failed to decode cursor: %v", err)
	}
	var result map[string]string
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("Failed to unmarshal cursor: %v", err)
	}
	return result
}

func TestListCountersCursorFormat(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	defer cleanupTestDB(t, db)

	tenantID := createTestTenant(t, db, "blog")
	createTestCounter(t, db, tenantID, "a", 0)
	createTestCounter(t, db, tenantID, "b", 0)

	handler := ListCountersHandler(db)

	ctx := &fasthttp.RequestCtx{}
	ctx.Request.SetRequestURI("/tenants/" + tenantID + "/counters?limit=1")
	ctx.Request.Header.SetMethod("GET")
	ctx.SetUserValue("tenant_id", tenantID)

	handler(ctx)

	var resp models.ListCountersResponse
	if err := json.Unmarshal(ctx.Response.Body(), &resp); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if resp.NextCursor == nil {
		t.Fatal("Expected next_cursor")
	}

	cursorData := decodeRawCursor(t, *resp.NextCursor)
	if cursorData["created_at"] == "" {
		t.Error("Cursor missing created_at field")
	}
	if cursorData["id"] == "" {
		t.Error("Cursor missing id field")
	}
}
