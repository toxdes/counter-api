-- Replace single-column tenant_id index with composite index for cursor-based pagination
DROP INDEX IF EXISTS idx_counters_tenant_id;
CREATE INDEX idx_counters_tenant_created_id ON counters(tenant_id, created_at, id);

INSERT INTO schema_migrations (version) VALUES (3);
