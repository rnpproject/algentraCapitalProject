from typing import Literal

from fastapi import APIRouter, Depends, File, HTTPException, Request, UploadFile
from fastapi.responses import FileResponse, JSONResponse
from pydantic import BaseModel, ConfigDict, Field
from sqlalchemy.orm import Session

from ...core.config import get_settings
from ...database import get_db
from ...services.payment_bot import MAX_PROOF_BYTES
from ...services.payment_service import (
    PaymentOrderConflict,
    PaymentOrderNotFound,
    cancel_order,
    create_order,
    get_order_proof,
    list_orders,
    retry_admin_notification,
    submit_payment_proof as store_payment_proof,
)
from ...services import midtrans_service
from .mt5 import require_client_id


router = APIRouter(prefix="/payments", tags=["payments"])
QRIS_UNAVAILABLE_DETAIL = "Pembayaran QRIS belum tersedia. Gunakan instruksi pembayaran manual pada pesanan."


class PaymentOrderCreate(BaseModel):
    model_config = ConfigDict(str_strip_whitespace=True)

    label: str = Field(min_length=1, max_length=80)
    server: str = Field(min_length=1, max_length=128)
    login: str = Field(min_length=1, max_length=32, pattern=r"^\d+$")
    plan: str = Field(pattern=r"^(ZERO|PRO|EXPERT|MANAGED|EA|EA_VPS[1-5])$")
    execution_mode: Literal["EA", "MANAGED"] = "EA"


@router.get("/orders")
def list_payment_orders(
    owner_id: int = Depends(require_client_id), db: Session = Depends(get_db)
) -> list[dict[str, object]]:
    return list_orders(db, owner_id)


@router.get("/manual-instructions")
def manual_payment_instructions(
    _owner_id: int = Depends(require_client_id),
) -> dict[str, str | None]:
    """Instruksi pembayaran manual untuk ditampilkan di halaman transaksi web."""
    return {"instructions": get_settings().payment_manual_instructions}


@router.get("/orders/{order_id}/proof")
def view_payment_proof(
    order_id: int,
    owner_id: int = Depends(require_client_id),
    db: Session = Depends(get_db),
) -> FileResponse:
    try:
        proof_path, media_type = get_order_proof(db, order_id, owner_id)
    except PaymentOrderNotFound as exc:
        raise HTTPException(status_code=404, detail=str(exc)) from exc
    return FileResponse(
        proof_path,
        media_type=media_type,
        headers={"Content-Disposition": "inline", "Cache-Control": "private, no-store"},
    )


@router.post("/orders")
def create_payment_order(
    payload: PaymentOrderCreate,
    owner_id: int = Depends(require_client_id),
    db: Session = Depends(get_db),
) -> dict[str, object]:
    try:
        if payload.execution_mode == "MANAGED" and get_settings().copier_worker_api_key is None:
            raise HTTPException(status_code=503, detail="Layanan managed copy belum dikonfigurasi di server.")
        return create_order(db, owner_id=owner_id, **payload.model_dump())
    except PaymentOrderNotFound as exc:
        raise HTTPException(status_code=404, detail=str(exc)) from exc
    except PaymentOrderConflict as exc:
        raise HTTPException(status_code=409, detail=str(exc)) from exc


@router.post("/orders/{order_id}/cancel")
def cancel_payment_order(
    order_id: int,
    owner_id: int = Depends(require_client_id),
    db: Session = Depends(get_db),
) -> dict[str, object]:
    try:
        return cancel_order(db, order_id, owner_id)
    except PaymentOrderNotFound as exc:
        raise HTTPException(status_code=404, detail=str(exc)) from exc
    except PaymentOrderConflict as exc:
        raise HTTPException(status_code=409, detail=str(exc)) from exc


@router.post("/orders/{order_id}/proof")
async def submit_payment_proof(
    order_id: int,
    file: UploadFile = File(...),
    owner_id: int = Depends(require_client_id),
    db: Session = Depends(get_db),
) -> dict[str, object]:
    try:
        content = await file.read(MAX_PROOF_BYTES + 1)
        return await store_payment_proof(db, order_id, owner_id, content)
    except PaymentOrderNotFound as exc:
        raise HTTPException(status_code=404, detail=str(exc)) from exc
    except PaymentOrderConflict as exc:
        raise HTTPException(status_code=409, detail=str(exc)) from exc
    finally:
        await file.close()


@router.post("/orders/{order_id}/notify")
async def retry_admin_notification_endpoint(
    order_id: int,
    owner_id: int = Depends(require_client_id),
    db: Session = Depends(get_db),
) -> dict[str, bool]:
    try:
        return await retry_admin_notification(db, order_id, owner_id)
    except PaymentOrderNotFound as exc:
        raise HTTPException(status_code=404, detail=str(exc)) from exc
    except PaymentOrderConflict as exc:
        raise HTTPException(status_code=409, detail=str(exc)) from exc


@router.get("/qris.jpeg", include_in_schema=False)
def qris_image() -> dict[str, str]:
    raise HTTPException(status_code=410, detail=QRIS_UNAVAILABLE_DETAIL)


@router.post("/orders/{order_id}/qris")
def create_qris_payment(
    order_id: int,
    owner_id: int = Depends(require_client_id),
    db: Session = Depends(get_db),
) -> dict[str, object]:
    """Buat transaksi QRIS Midtrans untuk order. Mengembalikan URL kode QR."""
    try:
        return midtrans_service.create_qris_charge(db, order_id=order_id, owner_id=owner_id)
    except midtrans_service.MidtransNotConfigured as exc:
        raise HTTPException(status_code=503, detail=str(exc)) from exc
    except midtrans_service.MidtransOrderNotFound as exc:
        raise HTTPException(status_code=404, detail=str(exc)) from exc
    except midtrans_service.MidtransOrderConflict as exc:
        raise HTTPException(status_code=409, detail=str(exc)) from exc
    except midtrans_service.MidtransError as exc:
        raise HTTPException(status_code=502, detail=str(exc)) from exc


@router.get("/orders/{order_id}/qris")
def qris_payment_state(
    order_id: int,
    owner_id: int = Depends(require_client_id),
    db: Session = Depends(get_db),
) -> dict[str, object]:
    """Status QRIS untuk polling frontend."""
    state = midtrans_service.get_qris_state(db, order_id=order_id, owner_id=owner_id)
    if state is None:
        raise HTTPException(status_code=404, detail="Status QRIS tidak ditemukan")
    return state


@router.post("/midtrans/notification", include_in_schema=False)
async def midtrans_notification(request: Request, db: Session = Depends(get_db)) -> JSONResponse:
    """Webhook Midtrans. Tanpa auth BFF — diverifikasi via signature SHA512 Midtrans."""
    try:
        payload = await request.json()
    except Exception:
        return JSONResponse(status_code=400, content={"detail": "Payload tidak valid"})
    if not isinstance(payload, dict):
        return JSONResponse(status_code=400, content={"detail": "Payload tidak valid"})
    status, _ = midtrans_service.handle_notification(db, payload)
    if status == "INVALID_SIGNATURE":
        return JSONResponse(status_code=403, content={"detail": "Signature tidak valid"})
    return JSONResponse(status_code=200, content={"status": status})
