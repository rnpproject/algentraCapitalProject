"""HTTP endpoints for EA clients."""

import hmac

from fastapi import APIRouter, Depends, Header, HTTPException, Query
from sqlalchemy.orm import Session

from ...core.config import get_settings
from ...database import get_db
from ...schemas import CopySnapshotInput
from ...services.ea_service import (
    ExecutionReport,
    HeartbeatReport,
    TradeResultReport,
    heartbeat as process_heartbeat,
    pending as get_pending_signals,
    publish_master_snapshot as process_master_snapshot,
    report as process_report,
    result as process_result,
)

router = APIRouter(prefix="/ea", tags=["ea"])


def require_ea_key(x_api_key: str | None = Header(default=None, alias="X-API-Key")) -> None:
    configured = get_settings().ea_api_key
    if configured is None or not configured.get_secret_value():
        raise HTTPException(status_code=503, detail="EA_API_KEY belum dikonfigurasi")
    if not x_api_key or not hmac.compare_digest(x_api_key, configured.get_secret_value()):
        raise HTTPException(status_code=401, detail="API key EA tidak valid")


@router.post("/master/snapshot", dependencies=[Depends(require_ea_key)])
def publish_master_snapshot(payload: CopySnapshotInput, db: Session = Depends(get_db)) -> dict[str, object]:
    return process_master_snapshot(payload, db)


@router.get("/pending", dependencies=[Depends(require_ea_key)])
def pending(
    limit: int = Query(default=1, ge=1, le=10),
    db: Session = Depends(get_db),
) -> dict[str, object]:
    return get_pending_signals(limit, db)


@router.post("/report", dependencies=[Depends(require_ea_key)])
def report(payload: ExecutionReport, db: Session = Depends(get_db)) -> dict[str, object]:
    return process_report(payload, db)


@router.post("/result", dependencies=[Depends(require_ea_key)])
def result(payload: TradeResultReport, db: Session = Depends(get_db)) -> dict[str, object]:
    return process_result(payload, db)


@router.post("/heartbeat", dependencies=[Depends(require_ea_key)])
def heartbeat(payload: HeartbeatReport) -> dict[str, object]:
    return process_heartbeat(payload)
