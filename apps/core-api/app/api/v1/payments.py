from typing import Literal

from fastapi import APIRouter, Depends, HTTPException
from fastapi.responses import FileResponse
from pydantic import BaseModel, Field
from sqlalchemy.orm import Session

from ...database import get_db
from ...services.payment_service import (
    PaymentOrderConflict,
    PaymentOrderNotFound,
    cancel_order,
    get_order_proof,
    list_orders,
    retry_admin_notification,
)
from .mt5 import require_client_id


router = APIRouter(prefix="/payments", tags=["payments"])
PAYMENTS_UNAVAILABLE_DETAIL = (
    "Pembayaran QRIS segera tersedia dan belum dapat diproses. "
    "Jangan melakukan transfer atau mengunggah bukti pembayaran."
)


class PaymentOrderCreate(BaseModel):
    label: str = Field(min_length=1, max_length=80)
    server: str = Field(min_length=1, max_length=128)
    login: str = Field(min_length=1, max_length=32, pattern=r"^\d+$")
    plan: str = Field(pattern=r"^(ZERO|PRO|EXPERT)$")
    execution_mode: Literal["EA", "MANAGED"] = "EA"


@router.get("/orders")
def list_payment_orders(
    owner_id: int = Depends(require_client_id), db: Session = Depends(get_db)
) -> list[dict[str, object]]:
    return list_orders(db, owner_id)


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
) -> dict[str, object]:
    raise HTTPException(status_code=503, detail=PAYMENTS_UNAVAILABLE_DETAIL)


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
def submit_payment_proof(
    order_id: int,
    owner_id: int = Depends(require_client_id),
) -> dict[str, object]:
    raise HTTPException(status_code=503, detail=PAYMENTS_UNAVAILABLE_DETAIL)


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
    raise HTTPException(status_code=410, detail=PAYMENTS_UNAVAILABLE_DETAIL)
