package handlers

import (
	"counter/internal/database"
	"counter/internal/middleware"
	"counter/internal/utils"

	"github.com/google/uuid"
	"github.com/valyala/fasthttp"
)

// TenantAccess runs after route authorization and establishes the trusted
// database context. Store calls open scoped transactions only on cache misses
// or writes; cached counter reads do not acquire a database connection.
func TenantAccess(next fasthttp.RequestHandler) fasthttp.RequestHandler {
	return func(ctx *fasthttp.RequestCtx) {
		principal, ok := middleware.PrincipalFromRequest(ctx)
		if !ok {
			respondWithError(ctx, 401, "UNAUTHORIZED", "Authentication required")
			return
		}
		tenantID, _ := ctx.UserValue("tenant_id").(string)
		if tenantID != "" {
			if err := utils.ValidateUUID(tenantID); err != nil {
				respondWithError(ctx, 400, "INVALID_UUID", "Invalid tenant ID format")
				return
			}
			tenantID = uuid.MustParse(tenantID).String()
			ctx.SetUserValue("tenant_id", tenantID)
		}
		if !principal.IsAdmin && (tenantID == "" || tenantID != principal.TenantID) {
			respondWithError(ctx, 403, "FORBIDDEN", "Tenant access denied")
			return
		}
		access := database.Access{TenantID: tenantID, PrincipalID: principal.CredentialID, Admin: principal.IsAdmin}
		ctx.SetUserValue(transactionContextKey{}, database.WithAccess(requestDatabaseContext(ctx), access))
		defer ctx.RemoveUserValue(transactionContextKey{})
		next(ctx)
	}
}
