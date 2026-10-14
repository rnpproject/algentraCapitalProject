"""Client-facing payment order use cases."""

import logging
from pathlib import Path
import uuid

from sqlalchemy.orm import Session

from ..core.time_utils import wib_iso
from ..domain.client_plans import resolve_plan
from ..models import PaymentOrder
from ..repositories.payments import PaymentRepository
from .payment_bot import MAX_PROOF_BYTES, payment_proof_path, payment_review_bot


logger = logging.getLogger("copytrade.payments")


class PaymentOrderNotFound(LookupError):
    pass


class PaymentOrderConflict(RuntimeError):
    pass


def _plan_matches_mode(plan: str, execution_mode: str) -> bool:
    """Pastikan paket sesuai cara menjalankan (mencegah mode MANAGED harga EA)."""
    if plan == "MANAGED":
        return execution_mode == "MANAGED"
    if plan == "EA" or plan.startswith("EA_VPS"):
        return execution_mode == "EA"
    # Paket lama (ZERO/PRO/EXPERT): kompatibel mundur, mode bebas.
    return True


def create_order(
    db: Session,
    *,
    owner_id: int,
    label: str,
    server: str,
    login: str,
    plan: str,
    execution_mode: str,
) -> dict[str, object]:
    selected_plan = resolve_plan(plan)
    if selected_plan is None:
        raise PaymentOrderConflict("Paket layanan tidak dikenal")
    if not _plan_matches_mode(plan, execution_mode):
        raise PaymentOrderConflict("Paket tidak sesuai dengan cara menjalankan copy trading")
    order, failure = PaymentRepository(db).create_for_owner(
        owner_id=owner_id,
        label=label.strip(),
        server=server.strip(),
        login=login.strip(),
        plan=plan,
        execution_mode=execution_mode,
        total_idr=int(selected_plan["price_idr"]),
    )
    if failure == "MISSING":
        raise PaymentOrderNotFound("Akun pengguna tidak ditemukan")
    if failure == "ACCOUNT_LIMIT":
        raise PaymentOrderConflict("Anda sudah mencapai batas 10 akun Copy Trading.")
    if failure == "ACCOUNT_EXISTS":
        raise PaymentOrderConflict("Akun broker ini sudah terdaftar.")
    if failure == "ORDER_EXISTS":
        raise PaymentOrderConflict("Masih ada pesanan aktif untuk akun broker ini.")
    if order is None:
        raise RuntimeError("Pesanan pembayaran gagal disimpan")
    return _order_payload(order)


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
        "payment_method": order.payment_method,
        "midtrans_status": order.midtrans_status,
        "has_proof": order.proof_filename is not None,
        "account_id": order.account_id,
        "notification_sent": order.admin_notified_at is not None,
        "created_at": wib_iso(order.created_at),
        "updated_at": wib_iso(order.updated_at),
    }


def _proof_extension(content: bytes) -> str | None:
    if content.startswith(b"\xff\xd8\xff"):
        return ".jpg"
    if content.startswith(b"\x89PNG\r\n\x1a\n"):
        return ".png"
    if len(content) >= 12 and content.startswith(b"RIFF") and content[8:12] == b"WEBP":
        return ".webp"
    return None


async def submit_payment_proof(db: Session, order_id: int, owner_id: int, content: bytes) -> dict[str, object]:
    if not content:
        raise PaymentOrderConflict("File bukti pembayaran kosong.")
    if len(content) > MAX_PROOF_BYTES:
        raise PaymentOrderConflict("Ukuran bukti pembayaran maksimal 5 MB.")
    extension = _proof_extension(content)
    if extension is None:
        raise PaymentOrderConflict("Bukti pembayaran harus berupa gambar JPG, PNG, atau WebP yang valid.")

    filename = f"{uuid.uuid4().hex}{extension}"
    proof_path = payment_proof_path(filename)
    proof_path.parent.mkdir(parents=True, exist_ok=True)
    file_created = False
    try:
        with proof_path.open("xb") as proof_file:
            file_created = True
            proof_file.write(content)
        order, submitted = PaymentRepository(db).submit_proof_for_owner(order_id, owner_id, filename)
        if order is None:
            raise PaymentOrderNotFound("Pesanan tidak ditemukan")
        if not submitted:
            raise PaymentOrderConflict("Pesanan ini tidak sedang menunggu bukti pembayaran.")
    except Exception:
        if file_created:
            proof_path.unlink(missing_ok=True)
        raise
    notification = await payment_review_bot.notify_order(order_id)
    result = _order_payload(order)
    result["notification_sent"] = notification
    return result


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
