package handlers

import (
	"context"
	"counter/internal/models"
	"counter/internal/service"
	"testing"
	"time"

	"github.com/valyala/fasthttp"
)

type fakeTenantService struct {
	createdLabel string
	created      *models.Tenant
	createErr    error
	getID        string
	gotten       *models.Tenant
	getErr       error
}

func (f *fakeTenantService) Create(_ context.Context, label string) (*models.Tenant, error) {
	f.createdLabel = label
	return f.created, f.createErr
}

func (f *fakeTenantService) Get(_ context.Context, id string) (*models.Tenant, error) {
	f.getID = id
	return f.gotten, f.getErr
}

var _ service.TenantService = (*fakeTenantService)(nil)

func TestCreateTenantServiceHandlerUsesTenantService(t *testing.T) {
	now := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	fake := &fakeTenantService{
		created: &models.Tenant{ID: "tenant-id", Label: "acme", CreatedAt: now, UpdatedAt: now},
	}
	handler := CreateTenantServiceHandler(fake)

	ctx := &fasthttp.RequestCtx{}
	ctx.Request.Header.SetMethod(fasthttp.MethodPost)
	ctx.Request.SetBodyString(`{"label":"acme"}`)
	handler(ctx)

	if ctx.Response.StatusCode() != fasthttp.StatusCreated {
		t.Fatalf("status = %d, want %d", ctx.Response.StatusCode(), fasthttp.StatusCreated)
	}
	if fake.createdLabel != "acme" {
		t.Fatalf("service received label %q, want acme", fake.createdLabel)
	}
	if got := string(ctx.Response.Body()); got == "" {
		t.Fatal("expected tenant response body")
	}
}

func TestGetTenantServiceHandlerUsesTenantService(t *testing.T) {
	const tenantID = "123e4567-e89b-12d3-a456-426614174000"
	fake := &fakeTenantService{
		gotten: &models.Tenant{ID: tenantID, Label: "acme"},
	}
	handler := GetTenantServiceHandler(fake)

	ctx := &fasthttp.RequestCtx{}
	ctx.SetUserValue("tenant_id", tenantID)
	handler(ctx)

	if ctx.Response.StatusCode() != fasthttp.StatusOK {
		t.Fatalf("status = %d, want %d", ctx.Response.StatusCode(), fasthttp.StatusOK)
	}
	if fake.getID != tenantID {
		t.Fatalf("service received tenant ID %q, want %q", fake.getID, tenantID)
	}
}
