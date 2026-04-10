-- Remove unique constraint on counter labels per tenant
ALTER TABLE counters DROP CONSTRAINT counters_tenant_label_unique;
