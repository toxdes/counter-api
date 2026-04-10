-- Remove max_delta column from counters table
ALTER TABLE counters DROP COLUMN max_delta;

-- Untrack migration
DELETE FROM schema_migrations WHERE version = 3;
