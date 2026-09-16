package handlers

import (
	"counter/internal/models"
	"counter/internal/service"
	"counter/internal/store"
	"counter/internal/testutil"
	"encoding/json"
	"testing"
	"time"

	"github.com/valyala/fasthttp"
)

func TestOperationHistoryPaginationHasNoDuplicates(t *testing.T) {
	db := testutil.OpenPostgres(t)
	tenantID := createTestTenant(t, db, "history-blog")
	counterID := createTestCounter(t, db, tenantID, "likes", 3)
	createdAt := time.Date(2026, time.January, 3, 4, 5, 6, 0, time.UTC)
	operationIDs := []string{
		"123e4567-e89b-12d3-a456-426614174002",
		"123e4567-e89b-12d3-a456-426614174003",
		"123e4567-e89b-12d3-a456-426614174004",
	}
	for index, operationID := range operationIDs {
		if _, err := db.Exec(`
			INSERT INTO counter_operations (
				tenant_id, counter_id, operation_id, kind, delta, request_hash,
				value_before, value_after, state, created_at, completed_at
			) VALUES ($1, $2, $3, 'increment', 1, $4, $5, $6, 'completed', $7, $7)
		`, tenantID, counterID, operationID, []byte{byte(index + 1)}, index, index+1, createdAt); err != nil {
			t.Fatalf("insert operation %s: %v", operationID, err)
		}
	}

	handler := OperationHistoryServiceHandler(service.NewOperationHistoryService(store.NewCounterStore(db)))
	call := func(cursor string) models.OperationHistoryResponse {
		ctx := &fasthttp.RequestCtx{}
		uri := "/v2/tenants/" + tenantID + "/counters/" + counterID + "/operations?limit=2"
		if cursor != "" {
			uri += "&cursor=" + cursor
		}
		ctx.Request.SetRequestURI(uri)
		ctx.SetUserValue("tenant_id", tenantID)
		ctx.SetUserValue("counter_id", counterID)
		handler(ctx)
		if ctx.Response.StatusCode() != fasthttp.StatusOK {
			t.Fatalf("history status = %d: %s", ctx.Response.StatusCode(), ctx.Response.Body())
		}
		var response models.OperationHistoryResponse
		if err := json.Unmarshal(ctx.Response.Body(), &response); err != nil {
			t.Fatalf("decode history response: %v", err)
		}
		return response
	}

	first := call("")
	if len(first.Operations) != 2 || first.NextCursor == nil {
		t.Fatalf("first page = %#v", first)
	}
	second := call(*first.NextCursor)
	if len(second.Operations) != 1 || second.NextCursor != nil {
		t.Fatalf("second page = %#v", second)
	}
	if second.Operations[0].OperationID == first.Operations[0].OperationID || second.Operations[0].OperationID == first.Operations[1].OperationID {
		t.Fatalf("second page repeated an operation: %#v then %#v", first.Operations, second.Operations)
	}
}
