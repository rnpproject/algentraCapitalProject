ALTER TABLE app.payment_orders ADD COLUMN payment_method VARCHAR(16) NOT NULL DEFAULT 'MANUAL';

ALTER TABLE app.payment_orders ADD COLUMN midtrans_order_id VARCHAR(64);

ALTER TABLE app.payment_orders ADD COLUMN midtrans_transaction_id VARCHAR(64);

ALTER TABLE app.payment_orders ADD COLUMN midtrans_status VARCHAR(32);

CREATE UNIQUE INDEX ux_payment_orders_midtrans_order ON app.payment_orders (midtrans_order_id) WHERE midtrans_order_id IS NOT NULL;

COMMENT ON SCHEMA app IS 'algentra:revision:0003_midtrans_qris';

