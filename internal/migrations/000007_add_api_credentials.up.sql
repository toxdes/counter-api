CREATE TABLE api_credentials (
    id UUID PRIMARY KEY,
    tenant_id UUID REFERENCES tenants(id) ON DELETE CASCADE,
    verifier BYTEA NOT NULL,
    scopes TEXT[] NOT NULL DEFAULT '{}',
    is_admin BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMPTZ,
    revoked_at TIMESTAMPTZ,
    created_by TEXT NOT NULL DEFAULT '',
    rotated_from UUID REFERENCES api_credentials(id) ON DELETE SET NULL,
    CHECK (is_admin OR tenant_id IS NOT NULL)
);

CREATE INDEX idx_api_credentials_tenant
    ON api_credentials (tenant_id)
    WHERE revoked_at IS NULL;

CREATE TABLE auth_audit_events (
    id UUID PRIMARY KEY,
    credential_id UUID REFERENCES api_credentials(id) ON DELETE SET NULL,
    tenant_id UUID REFERENCES tenants(id) ON DELETE SET NULL,
    actor_id TEXT NOT NULL DEFAULT '',
    action TEXT NOT NULL CHECK (action IN ('created', 'rotated', 'revoked', 'privileged_adjustment')),
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_auth_audit_events_tenant_created
    ON auth_audit_events (tenant_id, created_at DESC);
