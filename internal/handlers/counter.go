package handlers

import (
	"counter/internal/cache"
	"counter/internal/database"
	"counter/internal/models"
	"counter/internal/utils"
	"encoding/base64"
	"encoding/json"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/valyala/fasthttp"
)

const (
	ErrorCodeDeltaExceedsMaximum = "DELTA_EXCEEDS_MAXIMUM"
)

// CreateCounterHandler handles counter creation requests
func CreateCounterHandler(db *database.DB) fasthttp.RequestHandler {
	return func(ctx *fasthttp.RequestCtx) {
		tenantID := ctx.UserValue("tenant_id").(string)

		// Validate UUID format
		if err := utils.ValidateUUID(tenantID); err != nil {
			respondWithError(ctx, fasthttp.StatusBadRequest, "INVALID_UUID", "Invalid tenant ID format")
			return
		}

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

		// Check if label already exists for this tenant
		var labelExists bool
		err = db.Get(&labelExists, "SELECT EXISTS(SELECT 1 FROM counters WHERE tenant_id = $1 AND label = $2)", tenantID, req.Label)
		if err != nil {
			respondWithError(ctx, fasthttp.StatusInternalServerError, "DATABASE_ERROR", "Database error")
			return
		}
		if labelExists {
			respondWithError(ctx, fasthttp.StatusConflict, "COUNTER_LABEL_EXISTS", "A counter with this label already exists for this tenant")
			return
		}

		// Create counter
		counterID := uuid.New().String()
		now := time.Now().UTC()

		_, err = db.Exec(
			"INSERT INTO counters (id, tenant_id, label, value, max_delta, created_at, updated_at) VALUES ($1, $2, $3, $4, $5, $6, $7)",
			counterID, tenantID, req.Label, req.InitialValue, req.MaxDelta, now, now,
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
			MaxDelta:  req.MaxDelta,
			CreatedAt: now,
			UpdatedAt: now,
		}

		respondWithJSON(ctx, fasthttp.StatusCreated, counter)
	}
}

// IncrementCounterHandler handles counter increment requests
func IncrementCounterHandler(db *database.DB) fasthttp.RequestHandler {
	return func(ctx *fasthttp.RequestCtx) {
		tenantID, ok := ctx.UserValue("tenant_id").(string)
		if !ok || tenantID == "" {
			respondWithError(ctx, fasthttp.StatusBadRequest, "INVALID_PARAMETER", "tenant_id is required")
			return
		}

		counterID, ok := ctx.UserValue("counter_id").(string)
		if !ok || counterID == "" {
			respondWithError(ctx, fasthttp.StatusBadRequest, "INVALID_PARAMETER", "counter_id is required")
			return
		}

		// Validate UUID formats
		if err := utils.ValidateUUID(tenantID); err != nil {
			respondWithError(ctx, fasthttp.StatusBadRequest, "INVALID_UUID", "Invalid tenant ID format")
			return
		}
		if err := utils.ValidateUUID(counterID); err != nil {
			respondWithError(ctx, fasthttp.StatusBadRequest, "INVALID_UUID", "Invalid counter ID format")
			return
		}

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

		// Fetch counter with max_delta to validate
		var counterData struct {
			MaxDelta int64 `db:"max_delta"`
		}
		err = db.Get(
			&counterData,
			"SELECT max_delta FROM counters WHERE id = $1 AND tenant_id = $2",
			counterID, tenantID,
		)
		if err != nil {
			respondWithError(ctx, fasthttp.StatusNotFound, "COUNTER_NOT_FOUND", "Counter not found")
			return
		}

		// Validate delta doesn't exceed max_delta
		if delta > counterData.MaxDelta {
			respondWithError(ctx, fasthttp.StatusBadRequest, ErrorCodeDeltaExceedsMaximum, "Delta exceeds maximum allowed value")
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
		tenantID, ok := ctx.UserValue("tenant_id").(string)
		if !ok || tenantID == "" {
			respondWithError(ctx, fasthttp.StatusBadRequest, "INVALID_PARAMETER", "tenant_id is required")
			return
		}

		counterID, ok := ctx.UserValue("counter_id").(string)
		if !ok || counterID == "" {
			respondWithError(ctx, fasthttp.StatusBadRequest, "INVALID_PARAMETER", "counter_id is required")
			return
		}

		// Validate UUID formats
		if err := utils.ValidateUUID(tenantID); err != nil {
			respondWithError(ctx, fasthttp.StatusBadRequest, "INVALID_UUID", "Invalid tenant ID format")
			return
		}
		if err := utils.ValidateUUID(counterID); err != nil {
			respondWithError(ctx, fasthttp.StatusBadRequest, "INVALID_UUID", "Invalid counter ID format")
			return
		}

		var req models.SetCounterValueRequest
		if err := json.Unmarshal(ctx.Request.Body(), &req); err != nil {
			respondWithError(ctx, fasthttp.StatusBadRequest, "INVALID_JSON", "Invalid JSON")
			return
		}

		// Validate request after unmarshaling
		if err := req.Validate(); err != nil {
			respondWithError(ctx, fasthttp.StatusBadRequest, "INVALID_PARAMETER", err.Error())
			return
		}

		// Double-check value is not nil (should be caught by Validate)
		if req.Value == nil {
			respondWithError(ctx, fasthttp.StatusBadRequest, "INVALID_PARAMETER", "value is required")
			return
		}

		// Verify counter exists and belongs to tenant
		var exists bool
		err := db.Get(&exists, "SELECT EXISTS(SELECT 1 FROM counters WHERE id = $1 AND tenant_id = $2)", counterID, tenantID)
		if err != nil || !exists {
			respondWithError(ctx, fasthttp.StatusNotFound, "COUNTER_NOT_FOUND", "Counter not found")
			return
		}

		// Set counter value
		now := time.Now().UTC()
		_, err = db.Exec(
			"UPDATE counters SET value = $1, updated_at = $2 WHERE id = $3",
			*req.Value, now, counterID,
		)
		if err != nil {
			respondWithError(ctx, fasthttp.StatusInternalServerError, "DATABASE_ERROR", "Failed to set counter value")
			return
		}

		resp := &models.SetValueResponse{
			CounterID: counterID,
			Value:     *req.Value,
			UpdatedAt: now,
		}

		respondWithJSON(ctx, fasthttp.StatusOK, resp)
	}
}

// GetCounterHandler handles counter retrieval requests
func GetCounterHandler(db *database.DB) fasthttp.RequestHandler {
	return func(ctx *fasthttp.RequestCtx) {
		tenantID, ok := ctx.UserValue("tenant_id").(string)
		if !ok || tenantID == "" {
			respondWithError(ctx, fasthttp.StatusBadRequest, "INVALID_PARAMETER", "tenant_id is required")
			return
		}

		counterID, ok := ctx.UserValue("counter_id").(string)
		if !ok || counterID == "" {
			respondWithError(ctx, fasthttp.StatusBadRequest, "INVALID_PARAMETER", "counter_id is required")
			return
		}

		// Validate UUID formats
		if err := utils.ValidateUUID(tenantID); err != nil {
			respondWithError(ctx, fasthttp.StatusBadRequest, "INVALID_UUID", "Invalid tenant ID format")
			return
		}
		if err := utils.ValidateUUID(counterID); err != nil {
			respondWithError(ctx, fasthttp.StatusBadRequest, "INVALID_UUID", "Invalid counter ID format")
			return
		}

		var counter models.Counter
		err := db.Get(
			&counter,
			"SELECT id, tenant_id, label, value, max_delta, created_at, updated_at FROM counters WHERE id = $1 AND tenant_id = $2",
			counterID, tenantID,
		)
		if err != nil {
			respondWithError(ctx, fasthttp.StatusNotFound, "COUNTER_NOT_FOUND", "Counter not found")
			return
		}

		respondWithJSON(ctx, fasthttp.StatusOK, &counter)
	}
}

type pageCursor struct {
	CreatedAt string `json:"created_at"`
	ID        string `json:"id"`
}

func encodePageCursor(c pageCursor) string {
	data, _ := json.Marshal(c)
	return base64.RawURLEncoding.EncodeToString(data)
}

func decodePageCursor(token string) (pageCursor, error) {
	data, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return pageCursor{}, err
	}
	var c pageCursor
	if err := json.Unmarshal(data, &c); err != nil {
		return pageCursor{}, err
	}
	return c, nil
}

// ListCountersHandler handles listing counters for a tenant with cursor-based pagination
func ListCountersHandler(db *database.DB) fasthttp.RequestHandler {
	return func(ctx *fasthttp.RequestCtx) {
		tenantID, ok := ctx.UserValue("tenant_id").(string)
		if !ok || tenantID == "" {
			respondWithError(ctx, fasthttp.StatusBadRequest, "INVALID_PARAMETER", "tenant_id is required")
			return
		}

		if err := utils.ValidateUUID(tenantID); err != nil {
			respondWithError(ctx, fasthttp.StatusBadRequest, "INVALID_UUID", "Invalid tenant ID format")
			return
		}

		var tenantExists bool
		err := db.Get(&tenantExists, "SELECT EXISTS(SELECT 1 FROM tenants WHERE id = $1)", tenantID)
		if err != nil || !tenantExists {
			respondWithError(ctx, fasthttp.StatusNotFound, "TENANT_NOT_FOUND", "Tenant not found")
			return
		}

		limit := 20
		if limitStr := string(ctx.QueryArgs().Peek("limit")); limitStr != "" {
			parsed, err := strconv.Atoi(limitStr)
			if err != nil || parsed < 1 || parsed > 100 {
				respondWithError(ctx, fasthttp.StatusBadRequest, "INVALID_PARAMETER", "limit must be between 1 and 100")
				return
			}
			limit = parsed
		}

		var cursorTime time.Time
		var cursorID string
		hasCursor := false

		if cursorStr := string(ctx.QueryArgs().Peek("cursor")); cursorStr != "" {
			decoded, err := decodePageCursor(cursorStr)
			if err != nil {
				respondWithError(ctx, fasthttp.StatusBadRequest, "INVALID_CURSOR", "Invalid cursor format")
				return
			}
			cursorTime, err = time.Parse(time.RFC3339Nano, decoded.CreatedAt)
			if err != nil {
				respondWithError(ctx, fasthttp.StatusBadRequest, "INVALID_CURSOR", "Invalid cursor timestamp")
				return
			}
			cursorID = decoded.ID
			hasCursor = true
		}

		var counters []models.Counter
		if hasCursor {
			err = db.Select(&counters, `
				SELECT id, tenant_id, label, value, max_delta, created_at, updated_at
				FROM counters
				WHERE tenant_id = $1 AND (created_at, id) > ($2, $3)
				ORDER BY created_at, id
				LIMIT $4
			`, tenantID, cursorTime, cursorID, limit+1)
		} else {
			err = db.Select(&counters, `
				SELECT id, tenant_id, label, value, max_delta, created_at, updated_at
				FROM counters
				WHERE tenant_id = $1
				ORDER BY created_at, id
				LIMIT $2
			`, tenantID, limit+1)
		}
		if err != nil {
			respondWithError(ctx, fasthttp.StatusInternalServerError, "DATABASE_ERROR", "Database error")
			return
		}

		resp := &models.ListCountersResponse{
			Counters: counters,
		}

		if len(counters) > limit {
			resp.Counters = counters[:limit]
			last := counters[limit-1]
			cursor := encodePageCursor(pageCursor{
				CreatedAt: last.CreatedAt.Format(time.RFC3339Nano),
				ID:        last.ID,
			})
			resp.NextCursor = &cursor
		}

		respondWithJSON(ctx, fasthttp.StatusOK, resp)
	}
}

// CachedGetCounterHandler handles counter retrieval requests with caching
func CachedGetCounterHandler(cachedCounter *cache.CachedCounter) fasthttp.RequestHandler {
	return func(ctx *fasthttp.RequestCtx) {
		tenantID, ok := ctx.UserValue("tenant_id").(string)
		if !ok || tenantID == "" {
			respondWithError(ctx, fasthttp.StatusBadRequest, "INVALID_PARAMETER", "tenant_id is required")
			return
		}

		counterID, ok := ctx.UserValue("counter_id").(string)
		if !ok || counterID == "" {
			respondWithError(ctx, fasthttp.StatusBadRequest, "INVALID_PARAMETER", "counter_id is required")
			return
		}

		// Validate UUID formats
		if err := utils.ValidateUUID(tenantID); err != nil {
			respondWithError(ctx, fasthttp.StatusBadRequest, "INVALID_UUID", "Invalid tenant ID format")
			return
		}
		if err := utils.ValidateUUID(counterID); err != nil {
			respondWithError(ctx, fasthttp.StatusBadRequest, "INVALID_UUID", "Invalid counter ID format")
			return
		}

		// Try to get from cache
		counter, err := cachedCounter.Get(tenantID, counterID)
		if err != nil {
			respondWithError(ctx, fasthttp.StatusNotFound, "COUNTER_NOT_FOUND", "Counter not found")
			return
		}

		respondWithJSON(ctx, fasthttp.StatusOK, counter)
	}
}

// CachedIncrementCounterHandler handles counter increment requests with caching
func CachedIncrementCounterHandler(cachedCounter *cache.CachedCounter) fasthttp.RequestHandler {
	return func(ctx *fasthttp.RequestCtx) {
		tenantID, ok := ctx.UserValue("tenant_id").(string)
		if !ok || tenantID == "" {
			respondWithError(ctx, fasthttp.StatusBadRequest, "INVALID_PARAMETER", "tenant_id is required")
			return
		}

		counterID, ok := ctx.UserValue("counter_id").(string)
		if !ok || counterID == "" {
			respondWithError(ctx, fasthttp.StatusBadRequest, "INVALID_PARAMETER", "counter_id is required")
			return
		}

		// Validate UUID formats
		if err := utils.ValidateUUID(tenantID); err != nil {
			respondWithError(ctx, fasthttp.StatusBadRequest, "INVALID_UUID", "Invalid tenant ID format")
			return
		}
		if err := utils.ValidateUUID(counterID); err != nil {
			respondWithError(ctx, fasthttp.StatusBadRequest, "INVALID_UUID", "Invalid counter ID format")
			return
		}

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

		// Get counter first to validate max_delta
		counter, err := cachedCounter.Get(tenantID, counterID)
		if err != nil {
			respondWithError(ctx, fasthttp.StatusNotFound, "COUNTER_NOT_FOUND", "Counter not found")
			return
		}

		// Validate delta doesn't exceed max_delta
		if delta > counter.MaxDelta {
			respondWithError(ctx, fasthttp.StatusBadRequest, ErrorCodeDeltaExceedsMaximum, "Delta exceeds maximum allowed value")
			return
		}

		// Increment asynchronously
		newValue, err := cachedCounter.IncrementAsync(tenantID, counterID, delta)
		if err != nil {
			respondWithError(ctx, fasthttp.StatusInternalServerError, "INCREMENT_FAILED", "Failed to increment counter")
			return
		}

		now := time.Now().UTC()
		resp := &models.IncrementResponse{
			CounterID: counterID,
			Value:     newValue,
			UpdatedAt: now,
		}

		respondWithJSON(ctx, fasthttp.StatusOK, resp)
	}
}

// CachedSetCounterValueHandler handles counter value set requests with caching
func CachedSetCounterValueHandler(cachedCounter *cache.CachedCounter) fasthttp.RequestHandler {
	return func(ctx *fasthttp.RequestCtx) {
		tenantID, ok := ctx.UserValue("tenant_id").(string)
		if !ok || tenantID == "" {
			respondWithError(ctx, fasthttp.StatusBadRequest, "INVALID_PARAMETER", "tenant_id is required")
			return
		}

		counterID, ok := ctx.UserValue("counter_id").(string)
		if !ok || counterID == "" {
			respondWithError(ctx, fasthttp.StatusBadRequest, "INVALID_PARAMETER", "counter_id is required")
			return
		}

		// Validate UUID formats
		if err := utils.ValidateUUID(tenantID); err != nil {
			respondWithError(ctx, fasthttp.StatusBadRequest, "INVALID_UUID", "Invalid tenant ID format")
			return
		}
		if err := utils.ValidateUUID(counterID); err != nil {
			respondWithError(ctx, fasthttp.StatusBadRequest, "INVALID_UUID", "Invalid counter ID format")
			return
		}

		var req models.SetCounterValueRequest
		if err := json.Unmarshal(ctx.Request.Body(), &req); err != nil {
			respondWithError(ctx, fasthttp.StatusBadRequest, "INVALID_JSON", "Invalid JSON")
			return
		}

		// Validate request after unmarshaling
		if err := req.Validate(); err != nil {
			respondWithError(ctx, fasthttp.StatusBadRequest, "INVALID_PARAMETER", err.Error())
			return
		}

		// Double-check value is not nil (should be caught by Validate)
		if req.Value == nil {
			respondWithError(ctx, fasthttp.StatusBadRequest, "INVALID_PARAMETER", "value is required")
			return
		}

		// Set value asynchronously
		err := cachedCounter.SetAsync(tenantID, counterID, *req.Value)
		if err != nil {
			respondWithError(ctx, fasthttp.StatusInternalServerError, "SET_FAILED", "Failed to set counter value")
			return
		}

		now := time.Now().UTC()
		resp := &models.SetValueResponse{
			CounterID: counterID,
			Value:     *req.Value,
			UpdatedAt: now,
		}

		respondWithJSON(ctx, fasthttp.StatusOK, resp)
	}
}
