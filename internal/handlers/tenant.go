package handlers

import (
	"context"
	"counter/internal/database"
	"counter/internal/models"
	"counter/internal/service"
	"counter/internal/store"
	"counter/internal/utils"
	"encoding/json"
	"errors"
	"github.com/valyala/fasthttp"
)

// CreateTenantHandler handles tenant creation requests
func CreateTenantHandler(db *database.DB) fasthttp.RequestHandler {
	return CreateTenantServiceHandler(service.NewTenantService(store.NewTenantStore(db)))
}

// CreateTenantServiceHandler handles tenant creation through the tenant service boundary.
func CreateTenantServiceHandler(tenantService service.TenantService) fasthttp.RequestHandler {
	return func(ctx *fasthttp.RequestCtx) {
		var req models.CreateTenantRequest
		if err := json.Unmarshal(ctx.Request.Body(), &req); err != nil {
			respondWithError(ctx, fasthttp.StatusBadRequest, "INVALID_JSON", "Invalid JSON")
			return
		}

		if err := req.Validate(); err != nil {
			respondWithError(ctx, fasthttp.StatusBadRequest, "INVALID_PARAMETER", err.Error())
			return
		}

		tenant, err := tenantService.Create(context.Background(), req.Label)
		if errors.Is(err, service.ErrConflict) {
			respondWithError(ctx, fasthttp.StatusConflict, "TENANT_LABEL_EXISTS", "A tenant with this label already exists")
			return
		}
		if err != nil {
			respondWithError(ctx, fasthttp.StatusInternalServerError, "DATABASE_ERROR", "Failed to create tenant")
			return
		}

		respondWithJSON(ctx, fasthttp.StatusCreated, tenant)
	}
}

// GetTenantHandler handles tenant retrieval requests
func GetTenantHandler(db *database.DB) fasthttp.RequestHandler {
	return GetTenantServiceHandler(service.NewTenantService(store.NewTenantStore(db)))
}

// GetTenantServiceHandler handles tenant retrieval through the tenant service boundary.
func GetTenantServiceHandler(tenantService service.TenantService) fasthttp.RequestHandler {
	return func(ctx *fasthttp.RequestCtx) {
		tenantID, ok := ctx.UserValue("tenant_id").(string)
		if !ok || tenantID == "" {
			respondWithError(ctx, fasthttp.StatusBadRequest, "INVALID_PARAMETER", "tenant_id is required")
			return
		}

		// Validate UUID format
		if err := utils.ValidateUUID(tenantID); err != nil {
			respondWithError(ctx, fasthttp.StatusBadRequest, "INVALID_UUID", "Invalid tenant ID format")
			return
		}

		tenant, err := tenantService.Get(context.Background(), tenantID)
		if errors.Is(err, service.ErrNotFound) {
			respondWithError(ctx, fasthttp.StatusNotFound, "TENANT_NOT_FOUND", "Tenant not found")
			return
		}
		if err != nil {
			respondWithError(ctx, fasthttp.StatusInternalServerError, "DATABASE_ERROR", "Database error")
			return
		}

		respondWithJSON(ctx, fasthttp.StatusOK, &tenant)
	}
}

func respondWithJSON(ctx *fasthttp.RequestCtx, status int, data interface{}) {
	ctx.Response.SetStatusCode(status)
	ctx.Response.Header.SetContentType("application/json")

	body, err := json.Marshal(data)
	if err != nil {
		ctx.Response.SetStatusCode(fasthttp.StatusInternalServerError)
		return
	}

	ctx.Response.SetBody(body)
}

func respondWithError(ctx *fasthttp.RequestCtx, status int, code, message string) {
	ctx.Response.SetStatusCode(status)
	ctx.Response.Header.SetContentType("application/json")

	errResp := NewErrorResponse(code, message)
	body, _ := json.Marshal(errResp)
	ctx.Response.SetBody(body)
}
