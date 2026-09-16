package handlers

import (
	"context"
	"counter/internal/models"
	"counter/internal/service"
	"testing"
	"time"

	"github.com/valyala/fasthttp"
)

type fakeCounterService struct {
	createdTenantID    string
	createdRequest     models.CreateCounterRequest
	created            *models.Counter
	createErr          error
	gottenTenantID     string
	gottenCounterID    string
	gotten             *models.Counter
	getErr             error
	listedTenantID     string
	listedCursor       *service.CounterCursor
	listedLimit        int
	page               service.CounterPage
	listErr            error
	incrementTenantID  string
	incrementCounterID string
	incrementDelta     int64
	incrementResult    *service.CounterMutationResult
	incrementErr       error
	setTenantID        string
	setCounterID       string
	setValue           int64
	setResult          *service.CounterMutationResult
	setErr             error
}

func (f *fakeCounterService) Create(_ context.Context, tenantID string, request models.CreateCounterRequest) (*models.Counter, error) {
	f.createdTenantID = tenantID
	f.createdRequest = request
	return f.created, f.createErr
}

func (f *fakeCounterService) Get(_ context.Context, tenantID, counterID string) (*models.Counter, error) {
	f.gottenTenantID = tenantID
	f.gottenCounterID = counterID
	return f.gotten, f.getErr
}

func (f *fakeCounterService) List(_ context.Context, tenantID string, cursor *service.CounterCursor, limit int) (service.CounterPage, error) {
	f.listedTenantID = tenantID
	f.listedCursor = cursor
	f.listedLimit = limit
	return f.page, f.listErr
}

func (f *fakeCounterService) Increment(_ context.Context, tenantID, counterID string, delta int64) (*service.CounterMutationResult, error) {
	f.incrementTenantID = tenantID
	f.incrementCounterID = counterID
	f.incrementDelta = delta
	return f.incrementResult, f.incrementErr
}

func (f *fakeCounterService) Set(_ context.Context, tenantID, counterID string, value int64) (*service.CounterMutationResult, error) {
	f.setTenantID = tenantID
	f.setCounterID = counterID
	f.setValue = value
	return f.setResult, f.setErr
}

var _ service.CounterService = (*fakeCounterService)(nil)

func TestCreateCounterServiceHandlerUsesCounterService(t *testing.T) {
	fake := &fakeCounterService{
		created: &models.Counter{ID: "counter-id", TenantID: "123e4567-e89b-12d3-a456-426614174000", Label: "likes"},
	}
	handler := CreateCounterServiceHandler(fake)
	const tenantID = "123e4567-e89b-12d3-a456-426614174000"

	ctx := &fasthttp.RequestCtx{}
	ctx.SetUserValue("tenant_id", tenantID)
	ctx.Request.SetBodyString(`{"label":"likes","initial_value":3,"max_delta":7}`)
	handler(ctx)

	if ctx.Response.StatusCode() != fasthttp.StatusCreated {
		t.Fatalf("status = %d, want %d", ctx.Response.StatusCode(), fasthttp.StatusCreated)
	}
	if fake.createdTenantID != tenantID || fake.createdRequest.InitialValue != 3 || fake.createdRequest.MaxDelta != 7 {
		t.Fatalf("service request = %#v for tenant %q", fake.createdRequest, fake.createdTenantID)
	}
}

func TestGetCounterServiceHandlerUsesCounterService(t *testing.T) {
	const tenantID = "123e4567-e89b-12d3-a456-426614174000"
	const counterID = "123e4567-e89b-12d3-a456-426614174001"
	fake := &fakeCounterService{gotten: &models.Counter{ID: counterID, TenantID: tenantID}}
	handler := GetCounterServiceHandler(fake)

	ctx := &fasthttp.RequestCtx{}
	ctx.SetUserValue("tenant_id", tenantID)
	ctx.SetUserValue("counter_id", counterID)
	handler(ctx)

	if ctx.Response.StatusCode() != fasthttp.StatusOK {
		t.Fatalf("status = %d, want %d", ctx.Response.StatusCode(), fasthttp.StatusOK)
	}
	if fake.gottenTenantID != tenantID || fake.gottenCounterID != counterID {
		t.Fatalf("service IDs = %q/%q", fake.gottenTenantID, fake.gottenCounterID)
	}
}

func TestListCountersServiceHandlerUsesTypedCursor(t *testing.T) {
	const tenantID = "123e4567-e89b-12d3-a456-426614174000"
	createdAt := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	fake := &fakeCounterService{page: service.CounterPage{
		Counters: []models.Counter{{ID: "counter-id", TenantID: tenantID}},
	}}
	handler := ListCountersServiceHandler(fake)

	ctx := &fasthttp.RequestCtx{}
	ctx.SetUserValue("tenant_id", tenantID)
	ctx.Request.SetRequestURI("/tenants/" + tenantID + "/counters?limit=7&cursor=" + encodePageCursor(pageCursor{CreatedAt: createdAt.Format(time.RFC3339Nano), ID: "cursor-id"}))
	handler(ctx)

	if ctx.Response.StatusCode() != fasthttp.StatusOK {
		t.Fatalf("status = %d, want %d", ctx.Response.StatusCode(), fasthttp.StatusOK)
	}
	if fake.listedTenantID != tenantID || fake.listedLimit != 7 || fake.listedCursor == nil || fake.listedCursor.ID != "cursor-id" || !fake.listedCursor.CreatedAt.Equal(createdAt) {
		t.Fatalf("service list request = tenant %q, cursor %#v, limit %d", fake.listedTenantID, fake.listedCursor, fake.listedLimit)
	}
}

func TestIncrementCounterServiceHandlerUsesCounterService(t *testing.T) {
	const tenantID = "123e4567-e89b-12d3-a456-426614174000"
	const counterID = "123e4567-e89b-12d3-a456-426614174001"
	fake := &fakeCounterService{incrementResult: &service.CounterMutationResult{CounterID: counterID, Value: 12, UpdatedAt: time.Now().UTC()}}
	handler := IncrementCounterServiceHandler(fake)

	ctx := &fasthttp.RequestCtx{}
	ctx.SetUserValue("tenant_id", tenantID)
	ctx.SetUserValue("counter_id", counterID)
	ctx.Request.SetRequestURI("/tenants/" + tenantID + "/counters/" + counterID + "/inc?delta=5")
	handler(ctx)

	if ctx.Response.StatusCode() != fasthttp.StatusOK {
		t.Fatalf("status = %d, want %d", ctx.Response.StatusCode(), fasthttp.StatusOK)
	}
	if fake.incrementTenantID != tenantID || fake.incrementCounterID != counterID || fake.incrementDelta != 5 {
		t.Fatalf("service increment request = %q/%q/%d", fake.incrementTenantID, fake.incrementCounterID, fake.incrementDelta)
	}
}

func TestSetCounterServiceHandlerUsesCounterService(t *testing.T) {
	const tenantID = "123e4567-e89b-12d3-a456-426614174000"
	const counterID = "123e4567-e89b-12d3-a456-426614174001"
	fake := &fakeCounterService{setResult: &service.CounterMutationResult{CounterID: counterID, Value: 42, UpdatedAt: time.Now().UTC()}}
	handler := SetCounterServiceHandler(fake)

	ctx := &fasthttp.RequestCtx{}
	ctx.SetUserValue("tenant_id", tenantID)
	ctx.SetUserValue("counter_id", counterID)
	ctx.Request.SetBodyString(`{"value":42}`)
	handler(ctx)

	if ctx.Response.StatusCode() != fasthttp.StatusOK {
		t.Fatalf("status = %d, want %d", ctx.Response.StatusCode(), fasthttp.StatusOK)
	}
	if fake.setTenantID != tenantID || fake.setCounterID != counterID || fake.setValue != 42 {
		t.Fatalf("service set request = %q/%q/%d", fake.setTenantID, fake.setCounterID, fake.setValue)
	}
}
