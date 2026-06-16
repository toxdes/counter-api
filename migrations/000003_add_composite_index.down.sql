DROP INDEX IF EXISTS idx_counters_tenant_created_id;
CREATE INDEX idx_counters_tenant_id ON counters(tenant_id);
