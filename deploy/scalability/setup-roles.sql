-- Disposable local topology credentials only. Runs on both fresh and existing
-- volumes before migration; the API never receives the migration connection.
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'counter_runtime') THEN
        CREATE ROLE counter_runtime LOGIN PASSWORD 'local-only-runtime';
    END IF;
END $$;
ALTER ROLE counter_runtime NOSUPERUSER NOBYPASSRLS NOCREATEDB NOCREATEROLE NOINHERIT;
REVOKE CREATE ON SCHEMA public FROM PUBLIC;
GRANT USAGE ON SCHEMA public TO counter_runtime;
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO counter_runtime;
ALTER DEFAULT PRIVILEGES FOR ROLE counter IN SCHEMA public
    GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO counter_runtime;
