package handlers

import (
	"counter/internal/service"
	"counter/internal/utils"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/valyala/fasthttp"
)

func MetadataHandler(metadata *service.MetadataService) fasthttp.RequestHandler {
	return func(ctx *fasthttp.RequestCtx) {
		ctx.Response.Header.Set("Cache-Control", "no-store")
		tenantID, _ := ctx.UserValue("tenant_id").(string)
		counterID, _ := ctx.UserValue("counter_id").(string)
		if utils.ValidateUUID(tenantID) != nil || utils.ValidateUUID(counterID) != nil {
			respondWithError(ctx, 400, "INVALID_UUID", "Valid tenant and counter IDs are required")
			return
		}
		if ctx.IsGet() {
			result, err := metadata.Get(requestDatabaseContext(ctx), tenantID, counterID)
			if err != nil {
				metadataError(ctx, err)
				return
			}
			ctx.Response.Header.Set("ETag", `"`+result.Version+`"`)
			respondWithJSON(ctx, 200, result)
			return
		}
		match := strings.TrimSpace(string(ctx.Request.Header.Peek("If-Match")))
		none := strings.TrimSpace(string(ctx.Request.Header.Peek("If-None-Match")))
		create := ctx.IsPut() && none == "*" && match == ""
		if match == "" && none == "" {
			respondWithError(ctx, 428, "PRECONDITION_REQUIRED", "Use If-None-Match: * to create or If-Match with the current ETag to modify metadata")
			return
		}
		expected := ""
		if !create {
			if none != "" || len(match) < 2 || match[0] != '"' || match[len(match)-1] != '"' {
				respondWithError(ctx, 400, "INVALID_PRECONDITION", "If-Match must contain one strong metadata ETag")
				return
			}
			version, err := uuid.Parse(match[1 : len(match)-1])
			if err != nil || version.String() != match[1:len(match)-1] {
				respondWithError(ctx, 400, "INVALID_PRECONDITION", "Invalid metadata ETag")
				return
			}
			expected = version.String()
		}
		if ctx.IsDelete() {
			if err := metadata.Delete(requestDatabaseContext(ctx), tenantID, counterID, expected); err != nil {
				metadataError(ctx, err)
				return
			}
			ctx.SetStatusCode(204)
			return
		}
		result, err := metadata.Put(requestDatabaseContext(ctx), tenantID, counterID, ctx.Request.Body(), expected, create)
		if err != nil {
			metadataError(ctx, err)
			return
		}
		ctx.Response.Header.Set("ETag", `"`+result.Version+`"`)
		status := 200
		if create {
			status = 201
		}
		respondWithJSON(ctx, status, result)
	}
}

func metadataError(ctx *fasthttp.RequestCtx, err error) {
	switch {
	case errors.Is(err, service.ErrCounterNotFound):
		respondWithError(ctx, 404, "COUNTER_NOT_FOUND", "Counter not found")
	case errors.Is(err, service.ErrMetadataNotFound):
		respondWithError(ctx, 404, "METADATA_NOT_FOUND", "Counter has no metadata")
	case errors.Is(err, service.ErrMetadataPrecondition):
		respondWithError(ctx, 412, "PRECONDITION_FAILED", "Metadata changed or no longer exists; fetch its current ETag")
	case errors.Is(err, service.ErrMetadataTooLarge):
		respondWithError(ctx, 413, "METADATA_TOO_LARGE", err.Error())
	case errors.Is(err, service.ErrInvalidMetadata):
		respondWithError(ctx, 400, "INVALID_METADATA", "Provide valid JSON representable as JSONB, with nesting at most 64 levels")
	default:
		respondWithServiceError(ctx, err, 503, "SERVICE_UNAVAILABLE", "Metadata service is temporarily unavailable")
	}
}
