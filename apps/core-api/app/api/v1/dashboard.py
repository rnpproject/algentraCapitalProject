import asyncio
import json
from datetime import datetime, timedelta, timezone

from fastapi import APIRouter, Depends, HTTPException, Query, Request
from fastapi.responses import StreamingResponse
from sqlalchemy import select
from sqlalchemy.orm import Session

from ...core.config import get_settings
from ...database import SessionLocal, get_db
from ...models import Signal, utc_now
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
from ...state import master_snapshot_state
from ...core.time_utils import wib_iso
from .tg import telegram_manager


router = APIRouter(tags=["dashboard"])


def _sse_message(payload: dict[str, object]) -> bytes:
    return f"event: dashboard\ndata: {json.dumps(payload, separators=(',', ':'), default=str)}\n\n".encode()


@router.get("/realtime/stream")
async def realtime_stream(request: Request) -> StreamingResponse:
    """Stream dashboard snapshots to browsers through the Vercel BFF."""
    async def events():
        while not await request.is_disconnected():
            def load_snapshot():
                with SessionLocal() as db:
                    payload = get_dashboard_stats(db)
                    payload.update(risk_state(db))
                    latest = db.execute(
                        select(Signal.id).order_by(Signal.created_at.desc(), Signal.id.desc()).limit(1)
                    ).scalar_one_or_none()
                return payload, latest

            payload, latest = await asyncio.to_thread(load_snapshot)
            telegram = await telegram_manager.status()
            settings = get_settings()
            payload.update({
                "telegram_connected": bool(telegram["connected"]),
                "telegram_status": telegram["state"],
                "telegram_account": telegram["account_name"],
                "gemini_configured": bool(settings.gemini_api_key and settings.gemini_api_key.get_secret_value()),
                "gemini_model": settings.gemini_model,
            })
            yield _sse_message({
                "stats": payload,
                "latest_signal_id": latest,
                "server_time": wib_iso(utc_now()),
            })
            await asyncio.sleep(2)

    return StreamingResponse(
        events(), media_type="text/event-stream",
        headers={"Cache-Control": "private, no-store, no-transform", "X-Accel-Buffering": "no"},
    )


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


@router.get("/mt5-live")
def mt5_live() -> dict[str, object]:
    snapshot = master_snapshot_state.current()
    if snapshot is None:
        return {
            "available": False,
            "online": False,
            "observed_at": None,
            "account": None,
            "positions": [],
            "market_quotes": [],
        }

    now = utc_now()
    observed_at = snapshot.observed_at
    if observed_at.tzinfo is None:
        observed_at = observed_at.replace(tzinfo=timezone.utc)
    online = now >= observed_at and now - observed_at <= timedelta(seconds=20)
    account = snapshot.account
    account_data = (
        {
            "balance": account.balance,
            "equity": account.equity,
            "floating_profit": account.floating_profit,
            "currency": account.currency,
            "trade_mode": account.trade_mode,
            "observed_at": wib_iso(account.observed_at),
        }
        if account is not None
        else None
    )
    market_quotes = json.loads(snapshot.market_quotes_json) if snapshot.market_quotes_json else []
    return {
        "available": True,
        "online": online,
        "observed_at": wib_iso(snapshot.observed_at),
        "account": account_data,
        "positions": [
            {
                "ticket": position.ticket,
                "symbol": position.symbol,
                "action": position.action,
                "lots": position.lots,
                "entry_price": position.entry_price,
                "sl": position.sl,
                "tp": position.tp,
            }
            for position in snapshot.positions
        ],
        "market_quotes": market_quotes,
    }


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
