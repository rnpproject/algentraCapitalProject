from fastapi import APIRouter, Depends, HTTPException
from pydantic import BaseModel, Field
from sqlalchemy.orm import Session
from telethon.errors import FloodWaitError, PhoneCodeExpiredError, PhoneCodeInvalidError, SessionPasswordNeededError

from ...database import get_db
from ...services.telegram_group_service import list_groups, save_groups
from ...services.telegram_client import TelegramManager


router = APIRouter(prefix="/telegram", tags=["telegram"])
telegram_manager = TelegramManager()


class SendCodeRequest(BaseModel):
    phone: str = Field(min_length=7, max_length=24)


class VerifyCodeRequest(BaseModel):
    code: str = Field(min_length=3, max_length=12)


class Verify2FARequest(BaseModel):
    password: str = Field(min_length=1, max_length=512)


class GroupSelection(BaseModel):
    chat_id: str = Field(min_length=1, max_length=128)
    enabled: bool
    alias: str | None = Field(default=None, max_length=255)


class GroupSelectionRequest(BaseModel):
    groups: list[GroupSelection] = Field(max_length=1000)


@router.get("/status")
async def status() -> dict[str, object]:
    return await telegram_manager.status()


@router.post("/send-code")
async def send_code(payload: SendCodeRequest) -> dict[str, object]:
    try:
        return await telegram_manager.send_code(payload.phone)
    except ValueError as exc:
        raise HTTPException(status_code=422, detail=str(exc)) from exc
    except FloodWaitError as exc:
        raise HTTPException(status_code=429, detail=f"Telegram meminta tunggu {exc.seconds} detik") from exc
    except Exception as exc:
        raise HTTPException(status_code=502, detail=f"Gagal meminta kode Telegram ({type(exc).__name__})") from exc


@router.post("/verify-code")
async def verify_code(payload: VerifyCodeRequest) -> dict[str, object]:
    try:
        return await telegram_manager.verify_code(payload.code)
    except SessionPasswordNeededError:
        return {"status": "two_factor_required", "requires_2fa": True}
    except (PhoneCodeExpiredError, PhoneCodeInvalidError) as exc:
        raise HTTPException(status_code=422, detail="Kode Telegram tidak valid atau sudah kedaluwarsa") from exc
    except ValueError as exc:
        raise HTTPException(status_code=409, detail=str(exc)) from exc
    except FloodWaitError as exc:
        raise HTTPException(status_code=429, detail=f"Telegram meminta tunggu {exc.seconds} detik") from exc
    except Exception as exc:
        raise HTTPException(status_code=502, detail=f"Verifikasi Telegram gagal ({type(exc).__name__})") from exc


@router.post("/verify-2fa")
async def verify_2fa(payload: Verify2FARequest) -> dict[str, object]:
    try:
        return await telegram_manager.verify_2fa(payload.password)
    except ValueError as exc:
        raise HTTPException(status_code=409, detail=str(exc)) from exc
    except FloodWaitError as exc:
        raise HTTPException(status_code=429, detail=f"Telegram meminta tunggu {exc.seconds} detik") from exc
    except Exception as exc:
        raise HTTPException(status_code=502, detail=f"Password 2FA Telegram gagal ({type(exc).__name__})") from exc


@router.post("/logout")
async def logout() -> dict[str, str]:
    try:
        return await telegram_manager.logout()
    except Exception as exc:
        raise HTTPException(status_code=502, detail=f"Logout Telegram gagal ({type(exc).__name__})") from exc


@router.post("/reconnect")
async def reconnect() -> dict[str, object]:
    try:
        return await telegram_manager.reconnect()
    except FloodWaitError as exc:
        raise HTTPException(status_code=429, detail=f"Telegram meminta tunggu {exc.seconds} detik") from exc
    except Exception as exc:
        raise HTTPException(status_code=502, detail=f"Reconnect Telegram gagal ({type(exc).__name__})") from exc


@router.get("/groups")
async def groups(db: Session = Depends(get_db)) -> list[dict[str, object]]:
    try:
        available = await telegram_manager.get_groups()
    except RuntimeError as exc:
        raise HTTPException(status_code=409, detail=str(exc)) from exc
    return list_groups(db, available)


@router.post("/groups/select")
async def select_groups(payload: GroupSelectionRequest, db: Session = Depends(get_db)) -> dict[str, object]:
    try:
        available = await telegram_manager.get_groups()
    except RuntimeError as exc:
        raise HTTPException(status_code=409, detail=str(exc)) from exc
    try:
        return save_groups(db, available, [item.model_dump() for item in payload.groups])
    except ValueError as exc:
        raise HTTPException(status_code=422, detail=str(exc)) from exc
