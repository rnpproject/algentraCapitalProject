from datetime import datetime

from fastapi import APIRouter, Depends, HTTPException, Query
from fastapi.responses import StreamingResponse
from sqlalchemy.orm import Session

from ...core.config import get_settings
from ...database import get_db
from ...schemas import SignalResponse
from ...services.dashboard_service import (
    export_dashboard_logs,
    export_dashboard_trades,
    list_dashboard_logs,
    list_dashboard_signals,
    list_dashboard_trades,
)
from ...services.risk_controls import risk_state
from ...services.stats_service import get_dashboard_stats, get_group_stats
from .tg import telegram_manager


router = APIRouter(tags=["dashboard"])


@router.get("/stats")
async def stats(db: Session = Depends(get_db)) -> dict[str, object]:
    payload: dict[str, object] = get_dashboard_stats(db)
    payload.update(risk_state(db))
    telegram = await telegram_manager.status()
    settings = get_settings()
    payload.update(
        {
            "telegram_connected": bool(telegram["connected"]),
            "telegram_status": telegram["state"],
            "telegram_account": telegram["account_name"],
            "gemini_configured": bool(settings.gemini_api_key and settings.gemini_api_key.get_secret_value()),
            "gemini_model": settings.gemini_model,
        }
    )
    return payload


@router.get("/signals", response_model=list[SignalResponse])
def signals(
    limit: int = Query(default=100, ge=1, le=500),
    offset: int = Query(default=0, ge=0),
    status: str | None = Query(default=None, min_length=1, max_length=32),
    group_id: str | None = Query(default=None, max_length=128),
    db: Session = Depends(get_db),
) -> list[dict[str, object]]:
    return list_dashboard_signals(db, limit=limit, offset=offset, status=status, group_id=group_id)


@router.get("/logs")
def logs(
    limit: int = Query(default=100, ge=1, le=500),
    offset: int = Query(default=0, ge=0),
    level: str | None = Query(default=None, min_length=1, max_length=16),
    search: str | None = Query(default=None, max_length=200),
    db: Session = Depends(get_db),
) -> list[dict[str, object]]:
    return list_dashboard_logs(db, limit=limit, offset=offset, level=level, search=search)


@router.get("/logs/export.csv")
def export_logs(
    level: str | None = Query(default=None, max_length=16),
    search: str | None = Query(default=None, max_length=200),
    db: Session = Depends(get_db),
) -> StreamingResponse:
    content = export_dashboard_logs(db, level=level, search=search)
    return StreamingResponse(
        iter([content]),
        media_type="text/csv",
        headers={"Content-Disposition": "attachment; filename=logs.csv"},
    )


@router.get("/groups/stats")
def group_stats(db: Session = Depends(get_db)) -> list[dict[str, object]]:
    return get_group_stats(db)


@router.get("/trades")
def trades(
    limit: int = Query(default=100, ge=1, le=500),
    offset: int = Query(default=0, ge=0),
    date_from: datetime | None = None,
    date_to: datetime | None = None,
    group_id: str | None = Query(default=None, max_length=128),
    symbol: str | None = Query(default=None, max_length=64),
    result: str | None = Query(default=None, max_length=16),
    db: Session = Depends(get_db),
) -> list[dict[str, object]]:
    try:
        return list_dashboard_trades(
            db,
            date_from=date_from,
            date_to=date_to,
            group_id=group_id,
            symbol=symbol,
            result=result,
            limit=limit,
            offset=offset,
        )
    except ValueError as exc:
        raise HTTPException(status_code=422, detail=str(exc)) from exc


@router.get("/trades/export.csv")
def export_trades(
    date_from: datetime | None = None,
    date_to: datetime | None = None,
    group_id: str | None = Query(default=None, max_length=128),
    symbol: str | None = Query(default=None, max_length=64),
    result: str | None = Query(default=None, max_length=16),
    db: Session = Depends(get_db),
) -> StreamingResponse:
    try:
        content = export_dashboard_trades(
            db,
            date_from=date_from,
            date_to=date_to,
            group_id=group_id,
            symbol=symbol,
            result=result,
        )
    except ValueError as exc:
        raise HTTPException(status_code=422, detail=str(exc)) from exc
    return StreamingResponse(
        iter([content]),
        media_type="text/csv",
        headers={"Content-Disposition": "attachment; filename=trades.csv"},
    )
