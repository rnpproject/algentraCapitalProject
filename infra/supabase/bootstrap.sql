-- Development-only Supabase role and marker bootstrap. Production fresh setup uses
-- fresh-production.sql instead. This file does not create application tables.
-- Run as the project postgres owner in SQL Editor and replace the development project ref.

DO $$
DECLARE
    existing_comment text;
BEGIN
    IF current_database() <> 'postgres' THEN
        RAISE EXCEPTION 'Connect to the Supabase postgres database before running this bootstrap';
    END IF;
    SELECT shobj_description(
        (SELECT oid FROM pg_database WHERE datname = current_database()),
        'pg_database'
    ) INTO existing_comment;
    -- Supabase assigns this default comment to its administrative database.
    -- Keep rejecting every other existing comment to avoid changing an
    -- already-marked project/environment without inspection.
    IF existing_comment IS NOT NULL
       AND existing_comment <> 'default administrative connection database' THEN
        RAISE EXCEPTION 'Database already has a comment; verify it before changing the Algentra project marker';
    END IF;
END
$$;

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'algentra_api') THEN
        CREATE ROLE algentra_api NOLOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION;
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'algentra_migrator') THEN
        CREATE ROLE algentra_migrator NOLOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION;
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'algentra_backup') THEN
        CREATE ROLE algentra_backup NOLOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION;
    END IF;
END
$$;

-- The Go migration runner's first revision creates schema app. These temporary grants are required only
-- for that first development migration, which creates app and public.alembic_version.
GRANT CREATE ON DATABASE postgres TO algentra_migrator;
REVOKE CREATE ON SCHEMA public FROM PUBLIC;
GRANT CREATE ON SCHEMA public TO algentra_migrator;

-- Replace these values with the project environment and its exact Supabase project ref.
COMMENT ON DATABASE postgres IS 'algentra:development:REPLACE_WITH_SUPABASE_PROJECT_REF';
