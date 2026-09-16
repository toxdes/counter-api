package handlers

import (
	"context"
	"counter/internal/contract"
	"counter/internal/models"
	"counter/internal/service"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/valyala/fasthttp"
)

const (
	defaultOperationHistoryLimit = 50
	maxOperationHistoryLimit     = 100
	maxOperationHistoryCursor    = 512
)

// OperationHistoryServiceHandler serves the V1-compatible implementation
// shape for internal callers; the public route is registered as V2.
func OperationHistoryServiceHandler(historyService service.OperationHistoryService) fasthttp.RequestHandler {
	return OperationHistoryServiceHandlerVersioned(historyService, contract.V2)
}

// OperationHistoryServiceHandlerVersioned serves tenant-scoped completed
// operation history with strict cursor and page-size validation.
func OperationHistoryServiceHandlerVersioned(historyService service.OperationHistoryService, _ contract.Version) fasthttp.RequestHandler {
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
		if err := validateHistoryUUIDs(tenantID, counterID); err != nil {
			respondWithError(ctx, fasthttp.StatusBadRequest, "INVALID_UUID", err.Error())
			return
		}

		limit, err := parseOperationHistoryLimit(ctx)
		if err != nil {
			respondWithError(ctx, fasthttp.StatusBadRequest, "INVALID_PARAMETER", err.Error())
			return
		}
		cursor, err := parseOperationHistoryCursor(string(ctx.QueryArgs().Peek("cursor")))
		if err != nil {
			respondWithError(ctx, fasthttp.StatusBadRequest, "INVALID_CURSOR", err.Error())
			return
		}

		page, err := historyService.List(context.Background(), tenantID, counterID, cursor, limit)
		if errors.Is(err, service.ErrCounterNotFound) {
			respondWithError(ctx, fasthttp.StatusNotFound, "COUNTER_NOT_FOUND", "Counter not found")
			return
		}
		if err != nil {
			respondWithError(ctx, fasthttp.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "Counter service is temporarily unavailable")
			return
		}

		var nextCursor *string
		if page.Next != nil {
			encoded := encodeOperationHistoryCursor(*page.Next)
			nextCursor = &encoded
		}
		respondWithJSON(ctx, fasthttp.StatusOK, &models.OperationHistoryResponse{
			Operations: page.Operations,
			NextCursor: nextCursor,
		})
	}
}

type operationHistoryCursorToken struct {
	CreatedAt   string `json:"created_at"`
	OperationID string `json:"operation_id"`
}

func parseOperationHistoryLimit(ctx *fasthttp.RequestCtx) (int, error) {
	value := string(ctx.QueryArgs().Peek("limit"))
	if value == "" {
		return defaultOperationHistoryLimit, nil
	}
	limit, err := strconv.Atoi(value)
	if err != nil || limit < 1 || limit > maxOperationHistoryLimit {
		return 0, errors.New("limit must be between 1 and 100")
	}
	return limit, nil
}

func parseOperationHistoryCursor(token string) (*models.OperationCursor, error) {
	if token == "" {
		return nil, nil
	}
	if len(token) > maxOperationHistoryCursor {
		return nil, errors.New("cursor is too large")
	}
	data, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return nil, errors.New("cursor is malformed")
	}
	var value operationHistoryCursorToken
	if err := json.Unmarshal(data, &value); err != nil || value.CreatedAt == "" || value.OperationID == "" {
		return nil, errors.New("cursor is malformed")
	}
	createdAt, err := time.Parse(time.RFC3339Nano, value.CreatedAt)
	if err != nil {
		return nil, errors.New("cursor timestamp is malformed")
	}
	if _, err := uuid.Parse(value.OperationID); err != nil {
		return nil, errors.New("cursor operation ID is malformed")
	}
	return &models.OperationCursor{CreatedAt: createdAt, OperationID: value.OperationID}, nil
}

func encodeOperationHistoryCursor(cursor models.OperationCursor) string {
	data, _ := json.Marshal(operationHistoryCursorToken{
		CreatedAt:   cursor.CreatedAt.UTC().Format(time.RFC3339Nano),
		OperationID: cursor.OperationID,
	})
	return base64.RawURLEncoding.EncodeToString(data)
}

func validateHistoryUUIDs(tenantID, counterID string) error {
	if _, err := uuid.Parse(strings.TrimSpace(tenantID)); err != nil {
		return errors.New("Invalid tenant ID format")
	}
	if _, err := uuid.Parse(strings.TrimSpace(counterID)); err != nil {
		return errors.New("Invalid counter ID format")
	}
	return nil
}
