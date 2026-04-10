package handlers

import (
	"counter/internal/database"
	"counter/internal/models"
	"counter/internal/utils"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/valyala/fasthttp"
)

// CreateTenantHandler handles tenant creation requests
func CreateTenantHandler(db *database.DB) fasthttp.RequestHandler {
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

		// Check if label already exists
		var exists bool
		err := db.Get(&exists, "SELECT EXISTS(SELECT 1 FROM tenants WHERE label = $1)", req.Label)
		if err != nil {
			respondWithError(ctx, fasthttp.StatusInternalServerError, "DATABASE_ERROR", "Database error")
			return
		}

		if exists {
			respondWithError(ctx, fasthttp.StatusConflict, "TENANT_LABEL_EXISTS", "A tenant with this label already exists")
			return
		}

		// Create tenant
		tenantID := uuid.New().String()
		now := time.Now().UTC()

		_, err = db.Exec(
			"INSERT INTO tenants (id, label, created_at, updated_at) VALUES ($1, $2, $3, $4)",
			tenantID, req.Label, now, now,
		)
		if err != nil {
			respondWithError(ctx, fasthttp.StatusInternalServerError, "DATABASE_ERROR", "Failed to create tenant")
			return
		}

		tenant := &models.Tenant{
			ID:        tenantID,
			Label:     req.Label,
			CreatedAt: now,
			UpdatedAt: now,
		}

		respondWithJSON(ctx, fasthttp.StatusCreated, tenant)
	}
}

// GetTenantHandler handles tenant retrieval requests
func GetTenantHandler(db *database.DB) fasthttp.RequestHandler {
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

		var tenant models.Tenant
		err := db.Get(
			&tenant,
			"SELECT id, label, created_at, updated_at FROM tenants WHERE id = $1",
			tenantID,
		)
		if err != nil {
			respondWithError(ctx, fasthttp.StatusNotFound, "TENANT_NOT_FOUND", "Tenant not found")
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
