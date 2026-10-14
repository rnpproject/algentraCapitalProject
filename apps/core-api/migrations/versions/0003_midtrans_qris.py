"""Add Midtrans QRIS columns to payment_orders."""

from alembic import op

revision = "0003_midtrans_qris"
down_revision = "0002_rls_and_token_redaction"
branch_labels = None
depends_on = None


def upgrade() -> None:
    op.execute(
        "ALTER TABLE app.payment_orders "
        "ADD COLUMN payment_method VARCHAR(16) NOT NULL DEFAULT 'MANUAL'"
    )
    op.execute("ALTER TABLE app.payment_orders ADD COLUMN midtrans_order_id VARCHAR(64)")
    op.execute("ALTER TABLE app.payment_orders ADD COLUMN midtrans_transaction_id VARCHAR(64)")
    op.execute("ALTER TABLE app.payment_orders ADD COLUMN midtrans_status VARCHAR(32)")
    op.execute(
        "CREATE UNIQUE INDEX ux_payment_orders_midtrans_order "
        "ON app.payment_orders (midtrans_order_id) "
        "WHERE midtrans_order_id IS NOT NULL"
    )
    op.execute("COMMENT ON SCHEMA app IS 'algentra:revision:0003_midtrans_qris'")


def downgrade() -> None:
    op.execute("DROP INDEX IF EXISTS app.ux_payment_orders_midtrans_order")
    op.execute("ALTER TABLE app.payment_orders DROP COLUMN midtrans_status")
    op.execute("ALTER TABLE app.payment_orders DROP COLUMN midtrans_transaction_id")
    op.execute("ALTER TABLE app.payment_orders DROP COLUMN midtrans_order_id")
    op.execute("ALTER TABLE app.payment_orders DROP COLUMN payment_method")
    op.execute("COMMENT ON SCHEMA app IS 'algentra:revision:0002_rls_and_token_redaction'")
