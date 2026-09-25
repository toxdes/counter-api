package handlers

import (
	"context"
	"counter/internal/contract"
	"counter/internal/models"
	"counter/internal/service"
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"

	"github.com/valyala/fasthttp"
)

type fakeOperationHistoryService struct {
	tenantID  string
	counterID string
	cursor    *service.OperationCursor
	limit     int
	page      service.OperationHistoryPage
	err       error
}

func (f *fakeOperationHistoryService) List(_ context.Context, tenantID, counterID string, cursor *service.OperationCursor, limit int) (service.OperationHistoryPage, error) {
	f.tenantID = tenantID
	f.counterID = counterID
	f.cursor = cursor
	f.limit = limit
	return f.page, f.err
}

var _ service.OperationHistoryService = (*fakeOperationHistoryService)(nil)

func TestOperationHistoryV2HandlerPaginatesScopedOperations(t *testing.T) {
	const tenantID = "123e4567-e89b-12d3-a456-426614174000"
	const counterID = "123e4567-e89b-12d3-a456-426614174001"
	const operationID = "123e4567-e89b-12d3-a456-426614174002"
	createdAt := time.Date(2026, time.January, 3, 4, 5, 6, 0, time.UTC)
	fake := &fakeOperationHistoryService{page: service.OperationHistoryPage{
		Operations: []models.CounterOperation{{
			OperationID: operationID,
			CounterID:   counterID,
			Kind:        "increment",
			Delta:       5,
			ValueBefore: int64Ptr(10),
			ValueAfter:  int64Ptr(15),
			CreatedAt:   createdAt,
		}},
		Next: &service.OperationCursor{CreatedAt: createdAt, OperationID: operationID},
	}}
	handler := OperationHistoryServiceHandlerVersioned(fake, contract.V2)

	cursorBytes, _ := json.Marshal(map[string]string{
		"created_at":   createdAt.Format(time.RFC3339Nano),
		"operation_id": operationID,
	})
	ctx := &fasthttp.RequestCtx{}
	ctx.SetUserValue("tenant_id", tenantID)
	ctx.SetUserValue("counter_id", counterID)
	ctx.Request.SetRequestURI("/v2/tenants/" + tenantID + "/counters/" + counterID + "/operations?limit=2&cursor=" + base64.RawURLEncoding.EncodeToString(cursorBytes))
	handler(ctx)

	if ctx.Response.StatusCode() != fasthttp.StatusOK {
		t.Fatalf("status = %d, want %d: %s", ctx.Response.StatusCode(), fasthttp.StatusOK, ctx.Response.Body())
	}
	if fake.tenantID != tenantID || fake.counterID != counterID || fake.limit != 2 || fake.cursor == nil || fake.cursor.OperationID != operationID || !fake.cursor.CreatedAt.Equal(createdAt) {
		t.Fatalf("service request = %q/%q/%d/%#v", fake.tenantID, fake.counterID, fake.limit, fake.cursor)
	}
	var response models.OperationHistoryResponse
	if err := json.Unmarshal(ctx.Response.Body(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(response.Operations) != 1 || response.Operations[0].Delta != 5 || response.Operations[0].ValueAfter == nil || *response.Operations[0].ValueAfter != 15 {
		t.Fatalf("operations = %#v", response.Operations)
	}
	if response.NextCursor == nil || *response.NextCursor == "" {
		t.Fatalf("next cursor = %#v", response.NextCursor)
	}
}

func int64Ptr(value int64) *int64 {
	return &value
}
