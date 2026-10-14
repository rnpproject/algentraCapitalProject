"""Midtrans QRIS automatic payments via Core API.

Alur:
1. Client membuat order seperti biasa, lalu meminta QRIS charge.
2. Backend memanggil Midtrans /v2/charge (payment_type=qris) dan menyimpan
   kode QR ke order.
3. Client menampilkan QR; Midtrans mengirim HTTP notification ke webhook.
4. Webhook diverifikasi (signature SHA512 + nominal + merchant + status order)
   lalu order di-approve otomatis — akun MT5 dibuat tanpa campur tangan admin.

Keamanan:
- Server Key tidak pernah keluar dari backend (hanya di .env).
- Webhook wajib lolos verifikasi signature_key Midtrans.
- Nominal notifikasi harus sama persis dengan total order.
- Hanya order PENDING_PAYMENT yang bisa di-approve; idempotent.
"""

import base64
import hashlib
import hmac
import logging
import time
from decimal import Decimal, InvalidOperation

import httpx
from sqlalchemy.orm import Session

from ..core.config import get_settings
from ..models import PaymentOrder
from ..repositories.payments import PaymentRepository
from .payment_review_service import approve_paid_order


logger = logging.getLogger("copytrade.midtrans")

SANDBOX_BASE_URL = "https://api.sandbox.midtrans.com"
PRODUCTION_BASE_URL = "https://api.midtrans.com"

SUCCESS_STATUSES = {"capture", "settlement"}
FAILURE_STATUSES = {"deny", "cancel", "expire", "failure"}


class MidtransError(RuntimeError):
    pass


class MidtransNotConfigured(MidtransError):
    pass


class MidtransOrderNotFound(LookupError):
    pass


class MidtransOrderConflict(RuntimeError):
    pass


def midtrans_configured() -> bool:
    settings = get_settings()
    return bool(settings.midtrans_server_key and settings.midtrans_server_key.get_secret_value())


def _base_url() -> str:
    return PRODUCTION_BASE_URL if get_settings().midtrans_is_production else SANDBOX_BASE_URL


def _auth_header() -> dict[str, str]:
    settings = get_settings()
    server_key = settings.midtrans_server_key
    if server_key is None:
        raise MidtransNotConfigured("Pembayaran QRIS belum dikonfigurasi.")
    raw = base64.b64encode(f"{server_key.get_secret_value()}:".encode("utf-8")).decode("ascii")
    return {"Authorization": f"Basic {raw}"}


def _expected_merchant_id() -> str | None:
    merchant_id = get_settings().midtrans_merchant_id
    return merchant_id.strip() if merchant_id and merchant_id.strip() else None


def create_qris_charge(db: Session, *, order_id: int, owner_id: int) -> dict[str, object]:
    """Buat transaksi QRIS Midtrans untuk order milik user. Mengembalikan URL kode QR."""
    if not midtrans_configured():
        raise MidtransNotConfigured("Pembayaran QRIS belum tersedia.")
    repository = PaymentRepository(db)
    order = repository.get_for_owner(order_id, owner_id)
    if order is None:
        raise MidtransOrderNotFound("Pesanan tidak ditemukan")
    if order.status != "PENDING_PAYMENT":
        raise MidtransOrderConflict("Pesanan ini tidak sedang menunggu pembayaran.")

    settings = get_settings()
    midtrans_order_id = f"ALG-{order.id}-{int(time.time())}"
    payload: dict[str, object] = {
        "payment_type": "qris",
        "transaction_details": {
            "order_id": midtrans_order_id,
            "gross_amount": order.total_idr,
        },
        "item_details": [
            {
                "id": order.plan,
                "price": order.total_idr,
                "quantity": 1,
                "name": f"Algentra Copy Trading ({order.plan})",
            }
        ],
        "customer_details": {"email": order.email},
        "custom_field1": str(order.id),
    }
    headers = {
        "Accept": "application/json",
        "Content-Type": "application/json",
        **_auth_header(),
    }
    notification_url = (settings.midtrans_notification_url or "").strip()
    if notification_url:
        headers["X-Override-Notification"] = notification_url

    try:
        response = httpx.post(
            f"{_base_url()}/v2/charge",
            json=payload,
            headers=headers,
            timeout=30.0,
        )
    except httpx.HTTPError as exc:
        logger.warning("Midtrans charge gagal terhubung (%s)", type(exc).__name__)
        raise MidtransError("Tidak dapat menghubungi Midtrans. Coba lagi sebentar.") from exc
    if response.status_code not in (200, 201):
        logger.warning("Midtrans charge ditolak (HTTP %s): %s", response.status_code, response.text[:300])
        raise MidtransError("Midtrans menolak pembuatan QRIS. Coba lagi sebentar.")
    data = response.json()
    qr_code_url = next(
        (action.get("url") for action in data.get("actions", []) if action.get("name") == "generate-qr-code"),
        None,
    )
    if not qr_code_url:
        logger.warning("Midtrans charge tanpa QR code: %s", str(data)[:300])
        raise MidtransError("Midtrans tidak mengembalikan kode QR.")

    order.payment_method = "QRIS"
    order.midtrans_order_id = midtrans_order_id
    order.midtrans_transaction_id = str(data.get("transaction_id") or "") or None
    order.midtrans_status = str(data.get("transaction_status") or "pending")
    db.commit()
    logger.info("QRIS charge dibuat untuk order %s (midtrans %s)", order.id, midtrans_order_id)
    return {
        "order_id": order.id,
        "midtrans_order_id": midtrans_order_id,
        "qr_code_url": qr_code_url,
        "gross_amount": order.total_idr,
    }


def verify_notification_signature(payload: dict[str, object]) -> bool:
    """Verifikasi signature_key notifikasi Midtrans: SHA512(order_id+status_code+gross_amount+ServerKey)."""
    settings = get_settings()
    server_key = settings.midtrans_server_key
    if server_key is None:
        return False
    try:
        order_id = str(payload["order_id"])
        status_code = str(payload["status_code"])
        gross_amount = str(payload["gross_amount"])
        signature_key = str(payload.get("signature_key") or "")
    except KeyError:
        return False
    expected = hashlib.sha512(
        f"{order_id}{status_code}{gross_amount}{server_key.get_secret_value()}".encode("utf-8")
    ).hexdigest()
    return hmac.compare_digest(expected, signature_key)


def _notification_amount_idr(payload: dict[str, object]) -> int | None:
    try:
        return int(Decimal(str(payload["gross_amount"])))
    except (InvalidOperation, ValueError, KeyError):
        return None


def handle_notification(db: Session, payload: dict[str, object]) -> tuple[str, int | None]:
    """Proses webhook Midtrans. Mengembalikan (status, account_id). Selalu idempotent."""
    if not verify_notification_signature(payload):
        logger.warning("Notifikasi Midtrans signature tidak valid (order %s)", payload.get("order_id"))
        return "INVALID_SIGNATURE", None

    midtrans_order_id = str(payload.get("order_id") or "")
    repository = PaymentRepository(db)
    order = repository.get_by_midtrans_order_id(midtrans_order_id)
    if order is None:
        logger.warning("Notifikasi Midtrans untuk order tak dikenal: %s", midtrans_order_id)
        return "UNKNOWN_ORDER", None

    amount_idr = _notification_amount_idr(payload)
    if amount_idr is None or amount_idr != order.total_idr:
        logger.warning(
            "Notifikasi Midtrans nominal tidak cocok (order %s): %s vs %s",
            order.id, payload.get("gross_amount"), order.total_idr,
        )
        return "AMOUNT_MISMATCH", None

    expected_merchant = _expected_merchant_id()
    merchant_id = str(payload.get("merchant_id") or "").strip()
    if expected_merchant and merchant_id != expected_merchant:
        logger.warning("Notifikasi Midtrans merchant_id tidak cocok (order %s)", order.id)
        return "MERCHANT_MISMATCH", None

    transaction_status = str(payload.get("transaction_status") or "")
    fraud_status = str(payload.get("fraud_status") or "accept")
    order.midtrans_status = transaction_status
    order.midtrans_transaction_id = str(payload.get("transaction_id") or order.midtrans_transaction_id)
    db.commit()

    if transaction_status in SUCCESS_STATUSES and fraud_status == "accept":
        status, account_id = approve_paid_order(db, order.id)
        logger.info("Notifikasi Midtrans %s: order %s -> %s", transaction_status, order.id, status)
        return status, account_id
    if transaction_status in FAILURE_STATUSES:
        if order.status == "PENDING_PAYMENT":
            # Kembalikan ke menunggu pembayaran agar user bisa coba lagi.
            order.payment_method = "MANUAL"
            order.midtrans_order_id = None
            order.midtrans_transaction_id = None
            order.midtrans_status = transaction_status
            db.commit()
        logger.info("Notifikasi Midtrans %s: order %s dibatalkan/dikedaluwarsa", transaction_status, order.id)
        return "PAYMENT_FAILED", None
    return "PENDING", None


def get_qris_state(db: Session, *, order_id: int, owner_id: int) -> dict[str, object] | None:
    """Status QRIS untuk polling frontend. None bila order tak ada/bukan QRIS."""
    repository = PaymentRepository(db)
    order: PaymentOrder | None = repository.get_for_owner(order_id, owner_id)
    if order is None or order.payment_method != "QRIS" or not order.midtrans_order_id:
        return None
    return {
        "order_id": order.id,
        "status": order.status,
        "midtrans_status": order.midtrans_status,
        "midtrans_order_id": order.midtrans_order_id,
    }
