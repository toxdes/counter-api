package handlers

import (
	"counter/internal/database"
	"counter/internal/models"
	"encoding/json"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/valyala/fasthttp"
)

// CreateCounterHandler handles counter creation requests
func CreateCounterHandler(db *database.DB) fasthttp.RequestHandler {
	return func(ctx *fasthttp.RequestCtx) {
		tenantID := ctx.UserValue("tenant_id").(string)

		// Verify tenant exists
		var tenantExists bool
		err := db.Get(&tenantExists, "SELECT EXISTS(SELECT 1 FROM tenants WHERE id = $1)", tenantID)
		if err != nil || !tenantExists {
			respondWithError(ctx, fasthttp.StatusNotFound, "TENANT_NOT_FOUND", "Tenant not found")
			return
		}

		var req models.CreateCounterRequest
		if err := json.Unmarshal(ctx.Request.Body(), &req); err != nil {
			respondWithError(ctx, fasthttp.StatusBadRequest, "INVALID_JSON", "Invalid JSON")
			return
		}

		if err := req.Validate(); err != nil {
			respondWithError(ctx, fasthttp.StatusBadRequest, "INVALID_PARAMETER", err.Error())
			return
		}

		// Create counter
		counterID := uuid.New().String()
		now := time.Now().UTC()

		_, err = db.Exec(
			"INSERT INTO counters (id, tenant_id, label, value, created_at, updated_at) VALUES ($1, $2, $3, $4, $5, $6)",
			counterID, tenantID, req.Label, req.InitialValue, now, now,
		)
		if err != nil {
			respondWithError(ctx, fasthttp.StatusInternalServerError, "DATABASE_ERROR", "Failed to create counter")
			return
		}

		counter := &models.Counter{
			ID:        counterID,
			TenantID:  tenantID,
			Label:     req.Label,
			Value:     req.InitialValue,
			CreatedAt: now,
			UpdatedAt: now,
		}

		respondWithJSON(ctx, fasthttp.StatusCreated, counter)
	}
}

// IncrementCounterHandler handles counter increment requests
func IncrementCounterHandler(db *database.DB) fasthttp.RequestHandler {
	return func(ctx *fasthttp.RequestCtx) {
		tenantID := ctx.UserValue("tenant_id").(string)
		counterID := ctx.UserValue("counter_id").(string)

		// Parse delta from query params
		delta := int64(1) // default
		if deltaStr := string(ctx.QueryArgs().Peek("delta")); deltaStr != "" {
			parsed, err := strconv.ParseInt(deltaStr, 10, 64)
			if err != nil || parsed <= 0 {
				respondWithError(ctx, fasthttp.StatusBadRequest, "INVALID_DELTA", "Delta must be a positive integer")
				return
			}
			delta = parsed
		}

		// Verify counter exists and belongs to tenant
		var exists bool
		err := db.Get(&exists, "SELECT EXISTS(SELECT 1 FROM counters WHERE id = $1 AND tenant_id = $2)", counterID, tenantID)
		if err != nil || !exists {
			respondWithError(ctx, fasthttp.StatusNotFound, "COUNTER_NOT_FOUND", "Counter not found")
			return
		}

		// Increment counter and get new value
		now := time.Now().UTC()
		var newValue int64
		err = db.QueryRow(
			"UPDATE counters SET value = value + $1, updated_at = $2 WHERE id = $3 RETURNING value",
			delta, now, counterID,
		).Scan(&newValue)
		if err != nil {
			respondWithError(ctx, fasthttp.StatusInternalServerError, "DATABASE_ERROR", "Failed to increment counter")
			return
		}

		resp := &models.IncrementResponse{
			CounterID: counterID,
			Value:     newValue,
			UpdatedAt: now,
		}

		respondWithJSON(ctx, fasthttp.StatusOK, resp)
	}
}

// SetCounterValueHandler handles counter value set requests
func SetCounterValueHandler(db *database.DB) fasthttp.RequestHandler {
	return func(ctx *fasthttp.RequestCtx) {
		tenantID := ctx.UserValue("tenant_id").(string)
		counterID := ctx.UserValue("counter_id").(string)

		// Parse value from query params
		valueStr := string(ctx.QueryArgs().Peek("val"))
		if valueStr == "" {
			respondWithError(ctx, fasthttp.StatusBadRequest, "INVALID_PARAMETER", "Value parameter is required")
			return
		}

		value, err := strconv.ParseInt(valueStr, 10, 64)
		if err != nil {
			respondWithError(ctx, fasthttp.StatusBadRequest, "INVALID_VALUE", "Value must be an integer")
			return
		}

		// Verify counter exists and belongs to tenant
		var exists bool
		err = db.Get(&exists, "SELECT EXISTS(SELECT 1 FROM counters WHERE id = $1 AND tenant_id = $2)", counterID, tenantID)
		if err != nil || !exists {
			respondWithError(ctx, fasthttp.StatusNotFound, "COUNTER_NOT_FOUND", "Counter not found")
			return
		}

		// Set counter value
		now := time.Now().UTC()
		_, err = db.Exec(
			"UPDATE counters SET value = $1, updated_at = $2 WHERE id = $3",
			value, now, counterID,
		)
		if err != nil {
			respondWithError(ctx, fasthttp.StatusInternalServerError, "DATABASE_ERROR", "Failed to set counter value")
			return
		}

		resp := &models.SetValueResponse{
			CounterID: counterID,
			Value:     value,
			UpdatedAt: now,
		}

		respondWithJSON(ctx, fasthttp.StatusOK, resp)
	}
}

// GetCounterHandler handles counter retrieval requests
func GetCounterHandler(db *database.DB) fasthttp.RequestHandler {
	return func(ctx *fasthttp.RequestCtx) {
		tenantID := ctx.UserValue("tenant_id").(string)
		counterID := ctx.UserValue("counter_id").(string)

		var counter models.Counter
		err := db.Get(
			&counter,
			"SELECT id, tenant_id, label, value, created_at, updated_at FROM counters WHERE id = $1 AND tenant_id = $2",
			counterID, tenantID,
		)
		if err != nil {
			respondWithError(ctx, fasthttp.StatusNotFound, "COUNTER_NOT_FOUND", "Counter not found")
			return
		}

		respondWithJSON(ctx, fasthttp.StatusOK, &counter)
	}
}
