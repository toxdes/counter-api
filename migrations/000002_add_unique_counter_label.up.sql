-- Add unique constraint on counter labels per tenant
ALTER TABLE counters ADD CONSTRAINT counters_tenant_label_unique UNIQUE (tenant_id, label);
