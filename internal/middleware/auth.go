package middleware

import (
	"context"
	"counter/internal/models"
	"counter/internal/security"
	"crypto/subtle"
	"errors"
	"time"

	"github.com/qiangxue/fasthttp-routing"
	"github.com/valyala/fasthttp"
)

const (
	ScopeTenantRead       = "tenant:read"
	ScopeCounterRead      = "counter:read"
	ScopeCounterCreate    = "counter:create"
	ScopeCounterIncrement = "counter:increment"
	ScopeCounterAdjust    = "counter:adjust"
	ScopeCounterHistory   = "counter:history"
)

var (
	ErrInvalidCredentials    = errors.New("invalid credentials")
	ErrCredentialUnavailable = errors.New("credential service unavailable")
)

// Principal is the authenticated identity attached to a request.
type Principal struct {
	CredentialID string
	TenantID     string
	Scopes       []string
	IsAdmin      bool
	Legacy       bool
}

func (p Principal) HasScope(scope string) bool {
	if p.IsAdmin {
		return true
	}
	for _, allowed := range p.Scopes {
		if allowed == scope {
			return true
		}
	}
	return false
}

type principalContextKey struct{}

// CredentialLookup is the read-only persistence boundary needed by request
// authentication. Authentication never stores or returns the raw secret.
type CredentialLookup interface {
	GetActiveCredential(context.Context, string) (*models.APICredential, error)
}

// APIKeyAuthenticator supports the legacy environment key and generated,
// tenant-scoped credentials.
type APIKeyAuthenticator struct {
	legacyKey string
	lookup    CredentialLookup
}

func NewAPIKeyAuthenticator(legacyKey string, lookup CredentialLookup) *APIKeyAuthenticator {
	return &APIKeyAuthenticator{legacyKey: legacyKey, lookup: lookup}
}

func (a *APIKeyAuthenticator) Authenticate(ctx context.Context, apiKey string) (Principal, error) {
	if apiKey == "" {
		return Principal{}, ErrInvalidCredentials
	}
	if a.legacyKey != "" && secureKeyEqual(apiKey, a.legacyKey) {
		return Principal{
			CredentialID: "legacy-env-admin",
			Scopes: []string{
				ScopeTenantRead,
				ScopeCounterRead,
				ScopeCounterCreate,
				ScopeCounterIncrement,
				ScopeCounterAdjust,
				ScopeCounterHistory,
			},
			IsAdmin: true,
			Legacy:  true,
		}, nil
	}

	credentialID, secret, err := security.ParseAPIKey(apiKey)
	if err != nil || a.lookup == nil {
		return Principal{}, ErrInvalidCredentials
	}
	credential, err := a.lookup.GetActiveCredential(ctx, credentialID)
	if err != nil {
		return Principal{}, ErrCredentialUnavailable
	}
	if credential == nil || credential.RevokedAt != nil || credential.ExpiresAt != nil && !credential.ExpiresAt.After(time.Now().UTC()) {
		return Principal{}, ErrInvalidCredentials
	}
	if subtle.ConstantTimeCompare(security.HashAPIKey(secret), credential.Verifier) != 1 {
		return Principal{}, ErrInvalidCredentials
	}
	tenantID := ""
	if credential.TenantID != nil {
		tenantID = *credential.TenantID
	}
	return Principal{
		CredentialID: credential.ID,
		TenantID:     tenantID,
		Scopes:       append([]string(nil), credential.Scopes...),
		IsAdmin:      credential.IsAdmin,
	}, nil
}

// AuthenticateRequest authenticates an optional API key before routing. Public
// endpoints continue without a principal; protected route wrappers enforce it.
func AuthenticateRequest(authenticator *APIKeyAuthenticator) func(fasthttp.RequestHandler) fasthttp.RequestHandler {
	return func(next fasthttp.RequestHandler) fasthttp.RequestHandler {
		return func(ctx *fasthttp.RequestCtx) {
			apiKey := string(ctx.Request.Header.Peek("X-API-Key"))
			if apiKey == "" {
				next(ctx)
				return
			}
			principal, err := authenticator.Authenticate(context.Background(), apiKey)
			if err != nil {
				if errors.Is(err, ErrCredentialUnavailable) {
					ctx.SetStatusCode(fasthttp.StatusServiceUnavailable)
					ctx.Response.Header.SetContentType("application/json")
					ctx.SetBodyString(`{"errors":[{"code":"SERVICE_UNAVAILABLE","message":"Authentication service is temporarily unavailable"}]}`)
					return
				}
				respondUnauthorized(ctx)
				return
			}
			ctx.SetUserValue(principalContextKey{}, principal)
			next(ctx)
		}
	}
}

func PrincipalFromRequest(ctx *fasthttp.RequestCtx) (Principal, bool) {
	principal, ok := ctx.UserValue(principalContextKey{}).(Principal)
	return principal, ok
}

// RequireScopesRouting enforces both action scope and URL tenant ownership.
func RequireScopesRouting(scopes ...string) func(routing.Handler) routing.Handler {
	return func(next routing.Handler) routing.Handler {
		return func(c *routing.Context) error {
			principal, ok := PrincipalFromRequest(c.RequestCtx)
			if !ok {
				respondUnauthorized(c.RequestCtx)
				return nil
			}
			if !PrincipalAuthorized(principal, c.Param("tenant_id"), scopes...) {
				respondForbidden(c.RequestCtx)
				return nil
			}
			return next(c)
		}
	}
}

// PrincipalAuthorized checks action permissions and, for non-admins, tenant
// ownership derived from the authenticated credential.
func PrincipalAuthorized(principal Principal, tenantID string, scopes ...string) bool {
	for _, scope := range scopes {
		if !principal.HasScope(scope) {
			return false
		}
	}
	return principal.IsAdmin || tenantID != "" && principal.TenantID == tenantID
}

func RequireAdminRouting(next routing.Handler) routing.Handler {
	return func(c *routing.Context) error {
		principal, ok := PrincipalFromRequest(c.RequestCtx)
		if !ok {
			respondUnauthorized(c.RequestCtx)
			return nil
		}
		if !principal.IsAdmin {
			respondForbidden(c.RequestCtx)
			return nil
		}
		return next(c)
	}
}

// APIKeyAuth remains as a compatibility wrapper for direct callers.
func APIKeyAuth(expectedKey string) func(fasthttp.RequestHandler) fasthttp.RequestHandler {
	return func(next fasthttp.RequestHandler) fasthttp.RequestHandler {
		return func(ctx *fasthttp.RequestCtx) {
			if secureKeyEqual(string(ctx.Request.Header.Peek("X-API-Key")), expectedKey) {
				next(ctx)
				return
			}
			respondUnauthorized(ctx)
		}
	}
}

// APIKeyAuthRouting remains as a compatibility wrapper for direct callers.
func APIKeyAuthRouting(expectedKey string) func(routing.Handler) routing.Handler {
	return func(next routing.Handler) routing.Handler {
		return func(c *routing.Context) error {
			if secureKeyEqual(string(c.RequestCtx.Request.Header.Peek("X-API-Key")), expectedKey) {
				return next(c)
			}
			respondUnauthorized(c.RequestCtx)
			return nil
		}
	}
}

func secureKeyEqual(provided, expected string) bool {
	providedDigest := security.HashAPIKey(provided)
	expectedDigest := security.HashAPIKey(expected)
	return subtle.ConstantTimeCompare(providedDigest, expectedDigest) == 1
}

func respondUnauthorized(ctx *fasthttp.RequestCtx) {
	ctx.SetStatusCode(fasthttp.StatusUnauthorized)
	ctx.Response.Header.SetContentType("application/json")
	ctx.SetBodyString(`{"errors":[{"code":"UNAUTHORIZED","message":"Invalid or missing API key"}]}`)
}

func respondForbidden(ctx *fasthttp.RequestCtx) {
	ctx.SetStatusCode(fasthttp.StatusForbidden)
	ctx.Response.Header.SetContentType("application/json")
	ctx.SetBodyString(`{"errors":[{"code":"FORBIDDEN","message":"Insufficient permission"}]}`)
}

func hashAPIKey(secret string) []byte {
	return security.HashAPIKey(secret)
}

func formatAPIKey(credentialID, secret string) string {
	return security.FormatAPIKey(credentialID, secret)
}
