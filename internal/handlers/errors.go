package handlers

import (
	"context"
	"counter/internal/middleware"
	"counter/internal/requestctx"
	"database/sql"
	"errors"

	"github.com/lib/pq"
	"github.com/valyala/fasthttp"
)

// ErrorDetail represents a single error in the error response
type ErrorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// ErrorResponse represents the standardized error response format
type ErrorResponse struct {
	Errors []ErrorDetail `json:"errors"`
}

// NewErrorResponse creates a new error response with a single error
func NewErrorResponse(code, message string) *ErrorResponse {
	return &ErrorResponse{
		Errors: []ErrorDetail{
			{Code: code, Message: message},
		},
	}
}

// MultiErrorResponse creates a new error response with multiple errors
func MultiErrorResponse(errors []ErrorDetail) *ErrorResponse {
	return &ErrorResponse{
		Errors: errors,
	}
}

// requestDatabaseContext keeps direct handler tests and older integrations
// working while production requests receive the bounded database budget from
// the middleware layer.
func requestDatabaseContext(ctx *fasthttp.RequestCtx) context.Context {
	requestContext, _ := middleware.DatabaseContextFromRequest(ctx)
	return requestctx.WithRequestID(requestContext, middleware.RequestIDFromRequest(ctx))
}

func respondWithServiceError(ctx *fasthttp.RequestCtx, err error, fallbackStatus int, fallbackCode, fallbackMessage string) {
	middleware.RecordEvent(ctx, "service_error")
	if isUnavailable(err) {
		middleware.RecordEvent(ctx, "transaction_error")
		ctx.Response.Header.Set("Retry-After", "1")
		respondWithError(ctx, fasthttp.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "Service is temporarily unavailable")
		return
	}
	respondWithError(ctx, fallbackStatus, fallbackCode, fallbackMessage)
}

func isUnavailable(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) || errors.Is(err, sql.ErrConnDone) {
		return true
	}
	var postgresError *pq.Error
	if errors.As(err, &postgresError) {
		switch string(postgresError.Code) {
		case "53300", "55P03", "57014", "57P01":
			return true
		}
	}
	return false
}
