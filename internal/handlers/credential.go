package handlers

import (
	"context"
	"counter/internal/middleware"
	"counter/internal/models"
	"counter/internal/service"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"github.com/valyala/fasthttp"
)

func CreateCredentialServiceHandler(credentialService service.CredentialService) fasthttp.RequestHandler {
	return func(ctx *fasthttp.RequestCtx) {
		tenantID, ok := ctx.UserValue("tenant_id").(string)
		if !ok || tenantID == "" {
			respondWithError(ctx, fasthttp.StatusBadRequest, "INVALID_PARAMETER", "tenant_id is required")
			return
		}
		if err := validateCredentialTenantID(tenantID); err != nil {
			respondWithError(ctx, fasthttp.StatusBadRequest, "INVALID_UUID", err.Error())
			return
		}

		var request models.CredentialRequest
		if len(ctx.Request.Body()) > 0 {
			if err := json.Unmarshal(ctx.Request.Body(), &request); err != nil {
				respondWithError(ctx, fasthttp.StatusBadRequest, "INVALID_JSON", "Invalid JSON")
				return
			}
		}
		actorID := principalID(ctx)
		credential, err := credentialService.Create(context.Background(), tenantID, request.Scopes, request.ExpiresAt, actorID)
		if errors.Is(err, service.ErrTenantNotFound) {
			respondWithError(ctx, fasthttp.StatusNotFound, "TENANT_NOT_FOUND", "Tenant not found")
			return
		}
		if err != nil {
			respondWithError(ctx, fasthttp.StatusBadRequest, "INVALID_PARAMETER", err.Error())
			return
		}
		respondWithJSON(ctx, fasthttp.StatusCreated, credential)
	}
}

func CreateAdminCredentialServiceHandler(credentialService service.CredentialService) fasthttp.RequestHandler {
	return func(ctx *fasthttp.RequestCtx) {
		var request models.CredentialRequest
		if len(ctx.Request.Body()) > 0 {
			if err := json.Unmarshal(ctx.Request.Body(), &request); err != nil {
				respondWithError(ctx, fasthttp.StatusBadRequest, "INVALID_JSON", "Invalid JSON")
				return
			}
		}
		credential, err := credentialService.CreateAdmin(context.Background(), request.Scopes, request.ExpiresAt, principalID(ctx))
		if err != nil {
			respondWithError(ctx, fasthttp.StatusBadRequest, "INVALID_PARAMETER", err.Error())
			return
		}
		respondWithJSON(ctx, fasthttp.StatusCreated, credential)
	}
}

func RotateCredentialServiceHandler(credentialService service.CredentialService) fasthttp.RequestHandler {
	return func(ctx *fasthttp.RequestCtx) {
		tenantID, credentialID, ok := credentialRouteValues(ctx)
		if !ok {
			return
		}
		credential, err := credentialService.Rotate(context.Background(), tenantID, credentialID, principalID(ctx))
		if errors.Is(err, service.ErrCredentialNotFound) {
			respondWithError(ctx, fasthttp.StatusNotFound, "CREDENTIAL_NOT_FOUND", "Credential not found")
			return
		}
		if err != nil {
			respondWithError(ctx, fasthttp.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "Credential service is temporarily unavailable")
			return
		}
		respondWithJSON(ctx, fasthttp.StatusCreated, credential)
	}
}

func RevokeCredentialServiceHandler(credentialService service.CredentialService) fasthttp.RequestHandler {
	return func(ctx *fasthttp.RequestCtx) {
		tenantID, credentialID, ok := credentialRouteValues(ctx)
		if !ok {
			return
		}
		if err := credentialService.Revoke(context.Background(), tenantID, credentialID, principalID(ctx)); errors.Is(err, service.ErrCredentialNotFound) {
			respondWithError(ctx, fasthttp.StatusNotFound, "CREDENTIAL_NOT_FOUND", "Credential not found")
			return
		} else if err != nil {
			respondWithError(ctx, fasthttp.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "Credential service is temporarily unavailable")
			return
		}
		ctx.SetStatusCode(fasthttp.StatusNoContent)
	}
}

func credentialRouteValues(ctx *fasthttp.RequestCtx) (string, string, bool) {
	tenantID, ok := ctx.UserValue("tenant_id").(string)
	if !ok || tenantID == "" {
		respondWithError(ctx, fasthttp.StatusBadRequest, "INVALID_PARAMETER", "tenant_id is required")
		return "", "", false
	}
	credentialID, ok := ctx.UserValue("credential_id").(string)
	if !ok || credentialID == "" {
		respondWithError(ctx, fasthttp.StatusBadRequest, "INVALID_PARAMETER", "credential_id is required")
		return "", "", false
	}
	if err := validateCredentialTenantID(tenantID); err != nil {
		respondWithError(ctx, fasthttp.StatusBadRequest, "INVALID_UUID", err.Error())
		return "", "", false
	}
	if _, err := uuid.Parse(credentialID); err != nil {
		respondWithError(ctx, fasthttp.StatusBadRequest, "INVALID_UUID", "Invalid credential ID format")
		return "", "", false
	}
	return tenantID, credentialID, true
}

func principalID(ctx *fasthttp.RequestCtx) string {
	if principal, ok := middleware.PrincipalFromRequest(ctx); ok {
		return principal.CredentialID
	}
	return "anonymous"
}

func validateCredentialTenantID(value string) error {
	if _, err := uuid.Parse(value); err != nil {
		return errors.New("Invalid tenant ID format")
	}
	return nil
}
