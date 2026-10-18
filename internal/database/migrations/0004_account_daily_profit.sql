SET search_path TO app;

CREATE TABLE mt5_account_daily_profit (
    account_id INTEGER NOT NULL REFERENCES mt5_accounts(id) ON DELETE CASCADE,
    trade_date DATE NOT NULL,
    currency VARCHAR(16) NOT NULL,
    profit DOUBLE PRECISION NOT NULL DEFAULT 0,
    updated_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (account_id, trade_date)
);

CREATE TABLE mt5_master_daily_profit (
    trade_date DATE PRIMARY KEY,
    profit DOUBLE PRECISION NOT NULL DEFAULT 0,
    updated_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX ix_mt5_account_daily_profit_date ON mt5_account_daily_profit(account_id, trade_date DESC);

ALTER TABLE mt5_account_daily_profit ENABLE ROW LEVEL SECURITY;
CREATE POLICY algentra_api_full_access ON mt5_account_daily_profit FOR ALL TO algentra_api USING (true) WITH CHECK (true);
CREATE POLICY algentra_backup_read ON mt5_account_daily_profit FOR SELECT TO algentra_backup USING (true);
ALTER TABLE mt5_master_daily_profit ENABLE ROW LEVEL SECURITY;
CREATE POLICY algentra_api_full_access ON mt5_master_daily_profit FOR ALL TO algentra_api USING (true) WITH CHECK (true);
CREATE POLICY algentra_backup_read ON mt5_master_daily_profit FOR SELECT TO algentra_backup USING (true);

INSERT INTO mt5_account_daily_profit(account_id,trade_date,currency,profit,updated_at)
SELECT d.account_id,(d.deal_time AT TIME ZONE 'Asia/Jakarta')::date,COALESCE(s.currency,''),
       sum(d.profit+d.commission+d.swap+d.fee),CURRENT_TIMESTAMP
FROM mt5_history_deals d
LEFT JOIN mt5_account_state s ON s.account_id=d.account_id
GROUP BY d.account_id,(d.deal_time AT TIME ZONE 'Asia/Jakarta')::date,s.currency;

INSERT INTO mt5_master_daily_profit(trade_date,profit,updated_at)
SELECT (closed_at AT TIME ZONE 'Asia/Jakarta')::date,sum(profit),CURRENT_TIMESTAMP
FROM trades WHERE closed_at IS NOT NULL
GROUP BY (closed_at AT TIME ZONE 'Asia/Jakarta')::date;

COMMENT ON SCHEMA app IS 'algentra:revision:0004_account_daily_profit';
