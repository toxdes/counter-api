-- Drop counters table
DROP TABLE IF EXISTS counters;

-- Drop tenants table
DROP TABLE IF EXISTS tenants;

-- Remove migration tracking
DELETE FROM schema_migrations WHERE version = 1;
