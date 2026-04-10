-- Add max_delta column to counters table
ALTER TABLE counters ADD COLUMN max_delta BIGINT NOT NULL DEFAULT 50;

-- Track migration
INSERT INTO schema_migrations (version) VALUES (3);
