DROP POLICY credential_lookup ON api_credentials;
DROP POLICY credential_create ON api_credentials;
DROP POLICY credential_update ON api_credentials;
DROP POLICY credential_delete ON api_credentials;
ALTER TABLE api_credentials DISABLE ROW LEVEL SECURITY;
ALTER TABLE api_credentials NO FORCE ROW LEVEL SECURITY;

DROP POLICY request_isolation ON idempotency_requests;
ALTER TABLE idempotency_requests DISABLE ROW LEVEL SECURITY;
ALTER TABLE idempotency_requests NO FORCE ROW LEVEL SECURITY;
ALTER TABLE idempotency_requests DROP COLUMN response_headers, DROP COLUMN principal_id, DROP COLUMN tenant_id;

DROP POLICY tenant_isolation ON auth_audit_events;
ALTER TABLE auth_audit_events DISABLE ROW LEVEL SECURITY;
ALTER TABLE auth_audit_events NO FORCE ROW LEVEL SECURITY;
DROP POLICY tenant_isolation ON counter_metadata;
ALTER TABLE counter_metadata DISABLE ROW LEVEL SECURITY;
ALTER TABLE counter_metadata NO FORCE ROW LEVEL SECURITY;
DROP POLICY tenant_isolation ON counter_operations;
ALTER TABLE counter_operations DISABLE ROW LEVEL SECURITY;
ALTER TABLE counter_operations NO FORCE ROW LEVEL SECURITY;
DROP POLICY tenant_isolation ON counters;
ALTER TABLE counters DISABLE ROW LEVEL SECURITY;
ALTER TABLE counters NO FORCE ROW LEVEL SECURITY;
DROP POLICY tenant_isolation ON tenants;
ALTER TABLE tenants DISABLE ROW LEVEL SECURITY;
ALTER TABLE tenants NO FORCE ROW LEVEL SECURITY;
DROP FUNCTION counter_tenant_visible(uuid);
DROP FUNCTION counter_app_admin();
