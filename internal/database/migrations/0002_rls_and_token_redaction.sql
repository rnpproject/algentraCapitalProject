ALTER TABLE app.mt5_accounts DROP COLUMN token_ciphertext;

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

COMMENT ON SCHEMA app IS 'algentra:revision:0002_rls_and_token_redaction';

