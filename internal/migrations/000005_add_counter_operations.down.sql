DROP TABLE IF EXISTS counter_operations;

ALTER TABLE counters
    DROP CONSTRAINT IF EXISTS counters_tenant_id_id_unique;
