package handlers

import (
	"context"
	"counter/internal/cache"
	"counter/internal/database"
	"counter/internal/models"
	"counter/internal/service"
	"counter/internal/store"
	"counter/internal/utils"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strconv"
	"time"

	"github.com/valyala/fasthttp"
)

const (
	ErrorCodeDeltaExceedsMaximum = "DELTA_EXCEEDS_MAXIMUM"
)

// CreateCounterHandler handles counter creation requests
func CreateCounterHandler(db *database.DB) fasthttp.RequestHandler {
	return CreateCounterServiceHandler(service.NewCounterService(store.NewCounterStore(db)))
}

// CreateCounterServiceHandler handles counter creation through the counter service boundary.
func CreateCounterServiceHandler(counterService service.CounterService) fasthttp.RequestHandler {
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

		var req models.CreateCounterRequest
		if err := json.Unmarshal(ctx.Request.Body(), &req); err != nil {
			respondWithError(ctx, fasthttp.StatusBadRequest, "INVALID_JSON", "Invalid JSON")
			return
		}

		if err := req.Validate(); err != nil {
			respondWithError(ctx, fasthttp.StatusBadRequest, "INVALID_PARAMETER", err.Error())
			return
		}

		counter, err := counterService.Create(context.Background(), tenantID, req)
		if errors.Is(err, service.ErrTenantNotFound) {
			respondWithError(ctx, fasthttp.StatusNotFound, "TENANT_NOT_FOUND", "Tenant not found")
			return
		}
		if errors.Is(err, service.ErrConflict) {
			respondWithError(ctx, fasthttp.StatusConflict, "COUNTER_LABEL_EXISTS", "A counter with this label already exists for this tenant")
			return
		}
		if err != nil {
			respondWithError(ctx, fasthttp.StatusInternalServerError, "DATABASE_ERROR", "Failed to create counter")
			return
		}

		respondWithJSON(ctx, fasthttp.StatusCreated, counter)
	}
}

// IncrementCounterHandler handles counter increment requests
func IncrementCounterHandler(db *database.DB) fasthttp.RequestHandler {
	return IncrementCounterServiceHandler(service.NewCounterService(store.NewCounterStore(db)))
}

// IncrementCounterServiceHandler handles increments through the counter service boundary.
func IncrementCounterServiceHandler(counterService service.CounterService) fasthttp.RequestHandler {
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

		result, err := counterService.Increment(context.Background(), tenantID, counterID, delta)
		if errors.Is(err, service.ErrCounterNotFound) {
			respondWithError(ctx, fasthttp.StatusNotFound, "COUNTER_NOT_FOUND", "Counter not found")
			return
		}
		if errors.Is(err, service.ErrDeltaExceedsMaximum) {
			respondWithError(ctx, fasthttp.StatusBadRequest, ErrorCodeDeltaExceedsMaximum, "Delta exceeds maximum allowed value")
			return
		}
		if err != nil {
			respondWithError(ctx, fasthttp.StatusInternalServerError, "DATABASE_ERROR", "Failed to increment counter")
			return
		}

		resp := &models.IncrementResponse{
			CounterID: counterID,
			Value:     result.Value,
			UpdatedAt: result.UpdatedAt,
		}

		respondWithJSON(ctx, fasthttp.StatusOK, resp)
	}
}

// SetCounterValueHandler handles counter value set requests
func SetCounterValueHandler(db *database.DB) fasthttp.RequestHandler {
	return SetCounterServiceHandler(service.NewCounterService(store.NewCounterStore(db)))
}

// SetCounterServiceHandler handles counter value changes through the counter service boundary.
func SetCounterServiceHandler(counterService service.CounterService) fasthttp.RequestHandler {
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

		result, err := counterService.Set(context.Background(), tenantID, counterID, *req.Value)
		if errors.Is(err, service.ErrCounterNotFound) {
			respondWithError(ctx, fasthttp.StatusNotFound, "COUNTER_NOT_FOUND", "Counter not found")
			return
		}
		if err != nil {
			respondWithError(ctx, fasthttp.StatusInternalServerError, "DATABASE_ERROR", "Failed to set counter value")
			return
		}

		resp := &models.SetValueResponse{
			CounterID: counterID,
			Value:     result.Value,
			UpdatedAt: result.UpdatedAt,
		}

		respondWithJSON(ctx, fasthttp.StatusOK, resp)
	}
}

// GetCounterHandler handles counter retrieval requests
func GetCounterHandler(db *database.DB) fasthttp.RequestHandler {
	return GetCounterServiceHandler(service.NewCounterService(store.NewCounterStore(db)))
}

// GetCounterServiceHandler handles counter retrieval through the counter service boundary.
func GetCounterServiceHandler(counterService service.CounterService) fasthttp.RequestHandler {
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

		counter, err := counterService.Get(context.Background(), tenantID, counterID)
		if errors.Is(err, service.ErrCounterNotFound) {
			respondWithError(ctx, fasthttp.StatusNotFound, "COUNTER_NOT_FOUND", "Counter not found")
			return
		}
		if err != nil {
			respondWithError(ctx, fasthttp.StatusInternalServerError, "DATABASE_ERROR", "Database error")
			return
		}

		respondWithJSON(ctx, fasthttp.StatusOK, counter)
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
	return ListCountersServiceHandler(service.NewCounterService(store.NewCounterStore(db)))
}

// ListCountersServiceHandler handles counter listing through the counter service boundary.
func ListCountersServiceHandler(counterService service.CounterService) fasthttp.RequestHandler {
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

		var cursor *service.CounterCursor
		if hasCursor {
			cursor = &service.CounterCursor{CreatedAt: cursorTime, ID: cursorID}
		}
		page, err := counterService.List(context.Background(), tenantID, cursor, limit)
		if errors.Is(err, service.ErrTenantNotFound) {
			respondWithError(ctx, fasthttp.StatusNotFound, "TENANT_NOT_FOUND", "Tenant not found")
			return
		}
		if err != nil {
			respondWithError(ctx, fasthttp.StatusInternalServerError, "DATABASE_ERROR", "Database error")
			return
		}

		resp := &models.ListCountersResponse{
			Counters: page.Counters,
		}

		if page.Next != nil {
			last := page.Next
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
