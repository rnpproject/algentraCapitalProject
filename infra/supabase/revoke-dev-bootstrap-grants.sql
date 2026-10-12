-- Run once as the Supabase project owner after the first development migration.
-- Replace the ref below, then execute only against the marked development project.
DO $$
BEGIN
    IF current_database() <> 'postgres' OR shobj_description(
        (SELECT oid FROM pg_database WHERE datname = current_database()),
        'pg_database'
    ) IS DISTINCT FROM 'algentra:development:REPLACE_WITH_SUPABASE_PROJECT_REF' THEN
        RAISE EXCEPTION 'Development project marker mismatch; refusing to revoke grants';
    END IF;
END
$$;

REVOKE CREATE ON DATABASE postgres FROM algentra_migrator;
REVOKE CREATE ON SCHEMA public FROM algentra_migrator;
