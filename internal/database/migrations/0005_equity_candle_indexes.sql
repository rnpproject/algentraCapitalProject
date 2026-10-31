CREATE INDEX IF NOT EXISTS ix_mt5_master_equity_candle_minute
    ON app.mt5_master_equity_candles (minute_start);
CREATE INDEX IF NOT EXISTS ix_mt5_master_equity_candle_observed_at
    ON app.mt5_master_equity_candles (observed_at DESC);

COMMENT ON SCHEMA app IS 'algentra:revision:0005_equity_candle_indexes';
