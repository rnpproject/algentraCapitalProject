CREATE SCHEMA app AUTHORIZATION algentra_migrator;

REVOKE ALL ON SCHEMA app FROM PUBLIC;

GRANT USAGE ON SCHEMA app TO algentra_api, algentra_backup;

ALTER DEFAULT PRIVILEGES FOR ROLE algentra_migrator IN SCHEMA app GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO algentra_api;

ALTER DEFAULT PRIVILEGES FOR ROLE algentra_migrator IN SCHEMA app GRANT USAGE, SELECT, UPDATE ON SEQUENCES TO algentra_api;

ALTER DEFAULT PRIVILEGES FOR ROLE algentra_migrator IN SCHEMA app GRANT SELECT ON TABLES TO algentra_backup;

ALTER DEFAULT PRIVILEGES FOR ROLE algentra_migrator IN SCHEMA app GRANT SELECT ON SEQUENCES TO algentra_backup;

REVOKE ALL ON TABLE public.alembic_version FROM PUBLIC, algentra_api, algentra_backup;

ALTER TABLE public.alembic_version ENABLE ROW LEVEL SECURITY;

COMMENT ON SCHEMA app IS 'algentra:revision:0001_initial_postgres_schema';

SET search_path TO app;

CREATE TABLE admin_users (
    username VARCHAR(128) NOT NULL, 
    password_hash VARCHAR(128) NOT NULL, 
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (username)
);

CREATE TABLE auth_sessions (
    session_hash VARCHAR(64) NOT NULL, 
    user_type VARCHAR(16) NOT NULL, 
    user_id VARCHAR(128) NOT NULL, 
    session_data TEXT NOT NULL, 
    created_at TIMESTAMP WITH TIME ZONE NOT NULL, 
    last_seen_at TIMESTAMP WITH TIME ZONE NOT NULL, 
    expires_at TIMESTAMP WITH TIME ZONE NOT NULL, 
    revoked_at TIMESTAMP WITH TIME ZONE, 
    PRIMARY KEY (session_hash)
);

CREATE INDEX ix_auth_sessions_expiry ON auth_sessions (expires_at);

CREATE INDEX ix_auth_sessions_user ON auth_sessions (user_type, user_id, revoked_at);

CREATE TABLE client_users (
    id SERIAL NOT NULL, 
    email VARCHAR(320) NOT NULL, 
    password_hash VARCHAR(128) NOT NULL, 
    verified_at TIMESTAMP WITH TIME ZONE NOT NULL, 
    created_at TIMESTAMP WITH TIME ZONE NOT NULL, 
    PRIMARY KEY (id)
);

CREATE UNIQUE INDEX ux_client_users_email ON client_users (email);

CREATE TABLE ea_status (
    id SERIAL NOT NULL, 
    active BOOLEAN NOT NULL, 
    last_heartbeat TIMESTAMP WITH TIME ZONE, 
    terminal VARCHAR(128), 
    version VARCHAR(64), 
    symbol VARCHAR(64), 
    PRIMARY KEY (id)
);

CREATE TABLE email_verifications (
    email VARCHAR(320) NOT NULL, 
    code_hash VARCHAR(64) NOT NULL, 
    password_hash VARCHAR(128) NOT NULL, 
    expires_at TIMESTAMP WITH TIME ZONE NOT NULL, 
    resend_after TIMESTAMP WITH TIME ZONE NOT NULL, 
    attempts INTEGER NOT NULL, 
    send_count INTEGER NOT NULL, 
    send_window_started_at TIMESTAMP WITH TIME ZONE, 
    locked_until TIMESTAMP WITH TIME ZONE, 
    created_at TIMESTAMP WITH TIME ZONE NOT NULL, 
    PRIMARY KEY (email)
);

CREATE TABLE groups (
    chat_id VARCHAR(128) NOT NULL, 
    name VARCHAR(255) NOT NULL, 
    alias VARCHAR(255), 
    enabled BOOLEAN NOT NULL, 
    PRIMARY KEY (chat_id)
);

CREATE TABLE logs (
    id SERIAL NOT NULL, 
    level VARCHAR(16) NOT NULL, 
    source VARCHAR(64) NOT NULL, 
    message TEXT NOT NULL, 
    created_at TIMESTAMP WITH TIME ZONE NOT NULL, 
    PRIMARY KEY (id)
);

CREATE INDEX ix_logs_created_at ON logs (created_at);

CREATE TABLE mt5_master_account_state (
    id SERIAL NOT NULL, 
    balance FLOAT NOT NULL, 
    equity FLOAT NOT NULL, 
    floating_profit FLOAT NOT NULL, 
    currency VARCHAR(16) NOT NULL, 
    trade_mode VARCHAR(16) NOT NULL, 
    observed_at TIMESTAMP WITH TIME ZONE NOT NULL, 
    PRIMARY KEY (id)
);

CREATE TABLE mt5_master_copy_positions (
    id SERIAL NOT NULL, 
    source_ticket VARCHAR(64) NOT NULL, 
    symbol VARCHAR(64) NOT NULL, 
    action VARCHAR(8) NOT NULL, 
    lots FLOAT NOT NULL, 
    entry_price FLOAT NOT NULL, 
    sl FLOAT, 
    tp FLOAT, 
    is_open BOOLEAN NOT NULL, 
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL, 
    PRIMARY KEY (id), 
    CONSTRAINT ux_mt5_master_copy_source_ticket UNIQUE (source_ticket)
);

CREATE INDEX ix_mt5_master_copy_open ON mt5_master_copy_positions (is_open);

CREATE TABLE mt5_master_copy_state (
    id SERIAL NOT NULL, 
    last_snapshot_at TIMESTAMP WITH TIME ZONE, 
    PRIMARY KEY (id)
);

CREATE TABLE mt5_master_equity_candles (
    id SERIAL NOT NULL, 
    account_key VARCHAR(192) NOT NULL, 
    minute_start TIMESTAMP WITH TIME ZONE NOT NULL, 
    open_equity FLOAT NOT NULL, 
    high_equity FLOAT NOT NULL, 
    low_equity FLOAT NOT NULL, 
    close_equity FLOAT NOT NULL, 
    currency VARCHAR(16) NOT NULL, 
    observed_at TIMESTAMP WITH TIME ZONE NOT NULL, 
    PRIMARY KEY (id), 
    CONSTRAINT ux_mt5_master_equity_candle_account_minute UNIQUE (account_key, minute_start)
);

CREATE INDEX ix_mt5_master_equity_candle_account_minute ON mt5_master_equity_candles (account_key, minute_start);

CREATE TABLE mt5_master_equity_minute_samples (
    id SERIAL NOT NULL, 
    sample_minute TIMESTAMP WITH TIME ZONE NOT NULL, 
    equity FLOAT NOT NULL, 
    currency VARCHAR(16) NOT NULL, 
    observed_at TIMESTAMP WITH TIME ZONE NOT NULL, 
    PRIMARY KEY (id), 
    CONSTRAINT ux_mt5_master_equity_minute UNIQUE (sample_minute)
);

CREATE INDEX ix_mt5_master_equity_minute ON mt5_master_equity_minute_samples (sample_minute);

CREATE TABLE mt5_master_equity_samples (
    id SERIAL NOT NULL, 
    sample_hour TIMESTAMP WITH TIME ZONE NOT NULL, 
    equity FLOAT NOT NULL, 
    currency VARCHAR(16) NOT NULL, 
    observed_at TIMESTAMP WITH TIME ZONE NOT NULL, 
    PRIMARY KEY (id), 
    CONSTRAINT ux_mt5_master_equity_hour UNIQUE (sample_hour)
);

CREATE INDEX ix_mt5_master_equity_hour ON mt5_master_equity_samples (sample_hour);

CREATE TABLE mt5_master_market_state (
    id SERIAL NOT NULL, 
    quotes_json TEXT NOT NULL, 
    observed_at TIMESTAMP WITH TIME ZONE NOT NULL, 
    PRIMARY KEY (id)
);

CREATE TABLE password_reset_codes (
    email_hash VARCHAR(64) NOT NULL, 
    code_hash VARCHAR(64) NOT NULL, 
    expires_at TIMESTAMP WITH TIME ZONE NOT NULL, 
    resend_after TIMESTAMP WITH TIME ZONE NOT NULL, 
    attempts INTEGER NOT NULL, 
    send_count INTEGER NOT NULL, 
    send_window_started_at TIMESTAMP WITH TIME ZONE, 
    locked_until TIMESTAMP WITH TIME ZONE, 
    created_at TIMESTAMP WITH TIME ZONE NOT NULL, 
    last_activity_at TIMESTAMP WITH TIME ZONE NOT NULL, 
    PRIMARY KEY (email_hash)
);

CREATE INDEX ix_password_reset_last_activity ON password_reset_codes (last_activity_at);

CREATE TABLE settings (
    key VARCHAR(128) NOT NULL, 
    value TEXT NOT NULL, 
    PRIMARY KEY (key)
);

CREATE TABLE signals (
    id SERIAL NOT NULL, 
    group_id VARCHAR(128), 
    group_name VARCHAR(255), 
    message_id VARCHAR(128), 
    sender_id VARCHAR(128), 
    sender_name VARCHAR(255), 
    raw_text TEXT NOT NULL, 
    parsed_json TEXT, 
    normalized_text TEXT, 
    confidence FLOAT, 
    content_hash VARCHAR(64), 
    claimed_at TIMESTAMP WITH TIME ZONE, 
    status VARCHAR(32) NOT NULL, 
    created_at TIMESTAMP WITH TIME ZONE NOT NULL, 
    PRIMARY KEY (id)
);

CREATE INDEX ix_signals_created_at ON signals (created_at);

CREATE INDEX ix_signals_status_created_at ON signals (status, created_at);

CREATE UNIQUE INDEX ux_signals_content_hash ON signals (content_hash);

CREATE TABLE cs_conversations (
    id SERIAL NOT NULL, 
    client_user_id INTEGER, 
    public_visitor_hash VARCHAR(64), 
    started_at TIMESTAMP WITH TIME ZONE NOT NULL, 
    expires_at TIMESTAMP WITH TIME ZONE NOT NULL, 
    PRIMARY KEY (id), 
    CONSTRAINT ck_cs_conversation_identity CHECK ((client_user_id IS NOT NULL AND public_visitor_hash IS NULL) OR (client_user_id IS NULL AND public_visitor_hash IS NOT NULL)), 
    FOREIGN KEY(client_user_id) REFERENCES client_users (id) ON DELETE CASCADE
);

CREATE INDEX ix_cs_conversations_client_expiry ON cs_conversations (client_user_id, expires_at);

CREATE INDEX ix_cs_conversations_visitor_expiry ON cs_conversations (public_visitor_hash, expires_at);

CREATE TABLE ea_executions (
    id SERIAL NOT NULL, 
    signal_id INTEGER NOT NULL, 
    leg INTEGER NOT NULL, 
    status VARCHAR(16) NOT NULL, 
    ticket VARCHAR(128), 
    lots FLOAT, 
    exec_price FLOAT, 
    reason TEXT, 
    created_at TIMESTAMP WITH TIME ZONE NOT NULL, 
    PRIMARY KEY (id), 
    FOREIGN KEY(signal_id) REFERENCES signals (id), 
    CONSTRAINT ux_ea_executions_signal_leg UNIQUE (signal_id, leg)
);

CREATE TABLE mt5_accounts (
    id SERIAL NOT NULL, 
    owner_id INTEGER NOT NULL, 
    label VARCHAR(80) NOT NULL, 
    server VARCHAR(128) NOT NULL, 
    login VARCHAR(32) NOT NULL, 
    plan VARCHAR(16) NOT NULL, 
    role VARCHAR(16) NOT NULL, 
    execution_mode VARCHAR(16) NOT NULL, 
    token_hash VARCHAR(64) NOT NULL, 
    token_ciphertext TEXT, 
    active BOOLEAN NOT NULL, 
    last_seen_at TIMESTAMP WITH TIME ZONE, 
    created_at TIMESTAMP WITH TIME ZONE NOT NULL, 
    PRIMARY KEY (id), 
    FOREIGN KEY(owner_id) REFERENCES client_users (id) ON DELETE CASCADE, 
    CONSTRAINT ux_mt5_owner_server_login UNIQUE (owner_id, server, login)
);

CREATE INDEX ix_mt5_accounts_owner_role ON mt5_accounts (owner_id, role);

CREATE UNIQUE INDEX ux_mt5_accounts_token_hash ON mt5_accounts (token_hash);

CREATE TABLE trades (
    id SERIAL NOT NULL, 
    signal_id INTEGER, 
    ticket VARCHAR(128), 
    symbol VARCHAR(64), 
    action VARCHAR(16), 
    lots FLOAT, 
    entry FLOAT, 
    sl FLOAT, 
    tp TEXT, 
    exec_price FLOAT, 
    profit FLOAT NOT NULL, 
    result VARCHAR(16), 
    opened_at TIMESTAMP WITH TIME ZONE, 
    closed_at TIMESTAMP WITH TIME ZONE, 
    PRIMARY KEY (id), 
    FOREIGN KEY(signal_id) REFERENCES signals (id)
);

CREATE UNIQUE INDEX ux_trades_ticket ON trades (ticket);

CREATE TABLE cs_chat_messages (
    id SERIAL NOT NULL, 
    conversation_id INTEGER NOT NULL, 
    role VARCHAR(16) NOT NULL, 
    content TEXT NOT NULL, 
    created_at TIMESTAMP WITH TIME ZONE NOT NULL, 
    PRIMARY KEY (id), 
    FOREIGN KEY(conversation_id) REFERENCES cs_conversations (id) ON DELETE CASCADE
);

CREATE INDEX ix_cs_chat_messages_conversation_created ON cs_chat_messages (conversation_id, created_at);

CREATE TABLE managed_copy_profiles (
    account_id INTEGER NOT NULL, 
    password_ciphertext TEXT NOT NULL, 
    follower_symbol VARCHAR(64) NOT NULL, 
    mode_lot VARCHAR(16) NOT NULL, 
    ratio_lot FLOAT NOT NULL, 
    lot_tetap FLOAT NOT NULL, 
    max_lot_per_order FLOAT NOT NULL, 
    max_lot_total FLOAT NOT NULL, 
    max_open_positions INTEGER NOT NULL, 
    login_status VARCHAR(24) NOT NULL, 
    login_message VARCHAR(240), 
    active BOOLEAN NOT NULL, 
    status VARCHAR(24) NOT NULL, 
    status_message VARCHAR(240), 
    last_seen_at TIMESTAMP WITH TIME ZONE, 
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL, 
    PRIMARY KEY (account_id), 
    FOREIGN KEY(account_id) REFERENCES mt5_accounts (id) ON DELETE CASCADE
);

CREATE TABLE mt5_account_state (
    account_id INTEGER NOT NULL, 
    balance FLOAT NOT NULL, 
    equity FLOAT NOT NULL, 
    floating_profit FLOAT NOT NULL, 
    margin FLOAT NOT NULL, 
    free_margin FLOAT NOT NULL, 
    currency VARCHAR(16) NOT NULL, 
    trade_mode VARCHAR(16) NOT NULL, 
    allow_live_trading BOOLEAN NOT NULL, 
    terminal_trade_allowed BOOLEAN NOT NULL, 
    expert_trade_allowed BOOLEAN NOT NULL, 
    open_position_ids_json TEXT NOT NULL, 
    history_cursor VARCHAR(64) NOT NULL, 
    history_cursor_msc BIGINT NOT NULL, 
    observed_at TIMESTAMP WITH TIME ZONE NOT NULL, 
    PRIMARY KEY (account_id), 
    FOREIGN KEY(account_id) REFERENCES mt5_accounts (id) ON DELETE CASCADE
);

CREATE TABLE mt5_history_deals (
    id SERIAL NOT NULL, 
    account_id INTEGER NOT NULL, 
    deal_ticket VARCHAR(64) NOT NULL, 
    position_id VARCHAR(64) NOT NULL, 
    deal_time TIMESTAMP WITH TIME ZONE NOT NULL, 
    deal_time_msc BIGINT NOT NULL, 
    symbol VARCHAR(64) NOT NULL, 
    action VARCHAR(8) NOT NULL, 
    entry VARCHAR(12) NOT NULL, 
    volume FLOAT NOT NULL, 
    price FLOAT NOT NULL, 
    profit FLOAT NOT NULL, 
    commission FLOAT NOT NULL, 
    swap FLOAT NOT NULL, 
    fee FLOAT NOT NULL, 
    PRIMARY KEY (id), 
    FOREIGN KEY(account_id) REFERENCES mt5_accounts (id) ON DELETE CASCADE, 
    CONSTRAINT ux_mt5_history_account_ticket UNIQUE (account_id, deal_ticket)
);

CREATE INDEX ix_mt5_history_account_position ON mt5_history_deals (account_id, position_id);

CREATE INDEX ix_mt5_history_account_time ON mt5_history_deals (account_id, deal_time_msc);

CREATE TABLE payment_orders (
    id SERIAL NOT NULL, 
    owner_id INTEGER NOT NULL, 
    email VARCHAR(320) NOT NULL, 
    label VARCHAR(80) NOT NULL, 
    server VARCHAR(128) NOT NULL, 
    login VARCHAR(32) NOT NULL, 
    plan VARCHAR(16) NOT NULL, 
    execution_mode VARCHAR(16) NOT NULL, 
    total_idr INTEGER NOT NULL, 
    status VARCHAR(24) NOT NULL DEFAULT 'PENDING_PAYMENT',
    proof_filename VARCHAR(128), 
    account_id INTEGER, 
    admin_notified_at TIMESTAMP WITH TIME ZONE, 
    created_at TIMESTAMP WITH TIME ZONE NOT NULL, 
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL, 
    PRIMARY KEY (id), 
    FOREIGN KEY(account_id) REFERENCES mt5_accounts (id) ON DELETE SET NULL, 
    FOREIGN KEY(owner_id) REFERENCES client_users (id) ON DELETE CASCADE, 
    CONSTRAINT ux_payment_orders_account UNIQUE (account_id)
);

CREATE INDEX ix_payment_orders_owner_created ON payment_orders (owner_id, created_at);

CREATE INDEX ix_payment_orders_status_created ON payment_orders (status, created_at);

