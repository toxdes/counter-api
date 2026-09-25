-- Support deterministic cursor pagination by tenant and creation order.
CREATE INDEX idx_counters_tenant_created_id
    ON counters (tenant_id, created_at, id);
