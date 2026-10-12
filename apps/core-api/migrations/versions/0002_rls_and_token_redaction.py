"""Restrict Supabase tables to the two server-side application roles."""

from alembic import op

revision = "0002_rls_and_token_redaction"
down_revision = "0001_initial_postgres_schema"
branch_labels = None
depends_on = None


def upgrade() -> None:
    # Existing EA credentials remain valid through token_hash. Their reversible
    # ciphertext is obsolete now that chat can no longer disclose tokens.
    op.execute("ALTER TABLE app.mt5_accounts DROP COLUMN token_ciphertext")
    op.execute(
        """
        DO $$
        DECLARE table_name text;
        BEGIN
            FOR table_name IN
                SELECT tablename FROM pg_tables WHERE schemaname = 'app'
            LOOP
                EXECUTE format('ALTER TABLE app.%I ENABLE ROW LEVEL SECURITY', table_name);
                EXECUTE format(
                    'CREATE POLICY algentra_api_full_access ON app.%I '
                    'FOR ALL TO algentra_api USING (true) WITH CHECK (true)',
                    table_name
                );
                EXECUTE format(
                    'CREATE POLICY algentra_backup_read ON app.%I '
                    'FOR SELECT TO algentra_backup USING (true)',
                    table_name
                );
            END LOOP;
        END
        $$;
        """
    )
    op.execute("COMMENT ON SCHEMA app IS 'algentra:revision:0002_rls_and_token_redaction'")


def downgrade() -> None:
    op.execute(
        """
        DO $$
        DECLARE table_name text;
        BEGIN
            FOR table_name IN
                SELECT tablename FROM pg_tables WHERE schemaname = 'app'
            LOOP
                EXECUTE format('DROP POLICY IF EXISTS algentra_api_full_access ON app.%I', table_name);
                EXECUTE format('DROP POLICY IF EXISTS algentra_backup_read ON app.%I', table_name);
                EXECUTE format('ALTER TABLE app.%I DISABLE ROW LEVEL SECURITY', table_name);
            END LOOP;
        END
        $$;
        """
    )
    op.execute("ALTER TABLE app.mt5_accounts ADD COLUMN token_ciphertext text")
    op.execute("COMMENT ON SCHEMA app IS 'algentra:revision:0001_initial_postgres_schema'")
