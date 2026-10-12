"""Client-facing payment order use cases."""

import logging
from pathlib import Path

from sqlalchemy.orm import Session

from ..core.time_utils import wib_iso
from ..models import PaymentOrder
from ..repositories.payments import PaymentRepository
from .payment_bot import payment_proof_path, payment_review_bot


logger = logging.getLogger("copytrade.payments")


class PaymentOrderNotFound(LookupError):
    pass


class PaymentOrderConflict(RuntimeError):
    pass


def _order_payload(order: PaymentOrder) -> dict[str, object]:
    return {
        "id": order.id,
        "email": order.email,
        "label": order.label,
        "server": order.server,
        "login": order.login,
        "plan": order.plan,
        "execution_mode": order.execution_mode,
        "total_idr": order.total_idr,
        "status": order.status,
        "has_proof": order.proof_filename is not None,
        "account_id": order.account_id,
        "notification_sent": order.admin_notified_at is not None,
        "created_at": wib_iso(order.created_at),
        "updated_at": wib_iso(order.updated_at),
    }


def list_orders(db: Session, owner_id: int) -> list[dict[str, object]]:
    return [_order_payload(order) for order in PaymentRepository(db).list_for_owner(owner_id)]


def get_order_proof(db: Session, order_id: int, owner_id: int) -> tuple[Path, str]:
    order = PaymentRepository(db).get_for_owner(order_id, owner_id)
    if order is None or not order.proof_filename:
        raise PaymentOrderNotFound("Nota atau bukti pembayaran tidak ditemukan")
    proof_path = payment_proof_path(order.proof_filename)
    if not proof_path.is_file():
        raise PaymentOrderNotFound("File bukti pembayaran tidak tersedia")
    media_type = {".jpg": "image/jpeg", ".png": "image/png", ".webp": "image/webp"}.get(
        proof_path.suffix.casefold(), "application/octet-stream"
    )
    return proof_path, media_type


def cancel_order(db: Session, order_id: int, owner_id: int) -> dict[str, object]:
    order, old_proof_filename, cancelled = PaymentRepository(db).cancel_for_owner(order_id, owner_id)
    if order is None:
        raise PaymentOrderNotFound("Pesanan tidak ditemukan")
    if not cancelled:
        raise PaymentOrderConflict("Pesanan ini tidak bisa dibatalkan")
    if old_proof_filename is not None:
        try:
            payment_proof_path(old_proof_filename).unlink(missing_ok=True)
        except OSError as exc:
            logger.warning("Could not remove cancelled payment proof (%s)", type(exc).__name__)
    return _order_payload(order)


async def retry_admin_notification(db: Session, order_id: int, owner_id: int) -> dict[str, bool]:
    order = PaymentRepository(db).get_for_owner(order_id, owner_id)
    if order is None:
        raise PaymentOrderNotFound("Pesanan tidak ditemukan")
    if order.status != "PAYMENT_SUBMITTED":
        raise PaymentOrderConflict("Pesanan tidak sedang menunggu pemeriksaan pembayaran")
    if order.admin_notified_at is not None:
        return {"notification_sent": True}
    return {"notification_sent": await payment_review_bot.notify_order(order_id)}
