-- The application sets all context using set_config(..., true) inside each
-- transaction. Missing context denies access. The shared runtime role is a
-- trusted service identity; clients never get database credentials or set GUCs.
CREATE FUNCTION counter_app_admin() RETURNS boolean
LANGUAGE sql STABLE AS $$
    SELECT COALESCE(pg_catalog.current_setting('counter.is_admin', true), '') = 'true'
$$;

CREATE FUNCTION counter_tenant_visible(row_tenant uuid) RETURNS boolean
LANGUAGE sql STABLE AS $$
    SELECT row_tenant = NULLIF(pg_catalog.current_setting('counter.tenant_id', true), '')::uuid
       OR (counter_app_admin() AND NULLIF(pg_catalog.current_setting('counter.tenant_id', true), '') IS NULL)
$$;

ALTER TABLE idempotency_requests
    ADD COLUMN tenant_id UUID,
    ADD COLUMN principal_id TEXT NOT NULL DEFAULT '',
    ADD COLUMN response_headers JSONB NOT NULL DEFAULT '{}'::jsonb;

UPDATE idempotency_requests SET
    principal_id = split_part(scope, ':', 1),
    tenant_id = substring(scope FROM '/(?:v2/)?tenants/([0-9a-fA-F-]{36})(?:/|$)')::uuid;

ALTER TABLE tenants ENABLE ROW LEVEL SECURITY;
ALTER TABLE tenants FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON tenants
    USING (counter_tenant_visible(id)) WITH CHECK (counter_tenant_visible(id));

ALTER TABLE counters ENABLE ROW LEVEL SECURITY;
ALTER TABLE counters FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON counters
    USING (counter_tenant_visible(tenant_id)) WITH CHECK (counter_tenant_visible(tenant_id));

ALTER TABLE counter_operations ENABLE ROW LEVEL SECURITY;
ALTER TABLE counter_operations FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON counter_operations
    USING (counter_tenant_visible(tenant_id)) WITH CHECK (counter_tenant_visible(tenant_id));

ALTER TABLE counter_metadata ENABLE ROW LEVEL SECURITY;
ALTER TABLE counter_metadata FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON counter_metadata
    USING (counter_tenant_visible(tenant_id)) WITH CHECK (counter_tenant_visible(tenant_id));

ALTER TABLE auth_audit_events ENABLE ROW LEVEL SECURITY;
ALTER TABLE auth_audit_events FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON auth_audit_events
    USING (counter_tenant_visible(tenant_id)) WITH CHECK (counter_tenant_visible(tenant_id));

ALTER TABLE idempotency_requests ENABLE ROW LEVEL SECURITY;
ALTER TABLE idempotency_requests FORCE ROW LEVEL SECURITY;
CREATE POLICY request_isolation ON idempotency_requests
    USING (counter_tenant_visible(tenant_id)
        AND principal_id = pg_catalog.current_setting('counter.principal_id', true))
    WITH CHECK (counter_tenant_visible(tenant_id)
        AND principal_id = pg_catalog.current_setting('counter.principal_id', true));

ALTER TABLE api_credentials ENABLE ROW LEVEL SECURITY;
ALTER TABLE api_credentials FORCE ROW LEVEL SECURITY;
-- Pre-authentication can SELECT exactly one public credential ID. This grants
-- no write access and no access to tenant data or unrelated credentials.
CREATE POLICY credential_lookup ON api_credentials FOR SELECT
    USING (id = NULLIF(pg_catalog.current_setting('counter.auth_credential_id', true), '')::uuid
        OR (counter_app_admin() AND counter_tenant_visible(tenant_id)));
CREATE POLICY credential_create ON api_credentials FOR INSERT
    WITH CHECK (counter_app_admin() AND counter_tenant_visible(tenant_id));
CREATE POLICY credential_update ON api_credentials FOR UPDATE
    USING (counter_app_admin() AND counter_tenant_visible(tenant_id))
    WITH CHECK (counter_app_admin() AND counter_tenant_visible(tenant_id));
CREATE POLICY credential_delete ON api_credentials FOR DELETE
    USING (counter_app_admin() AND counter_tenant_visible(tenant_id));
