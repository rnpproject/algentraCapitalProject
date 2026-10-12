"""Dashboard use cases and response formatting."""

import csv
import io
from datetime import datetime

from sqlalchemy.orm import Session

from ..core.time_utils import wib_datetime_to_utc_naive, wib_iso
from ..models import Trade
from ..repositories.dashboard import DashboardRepository
from ..schemas import LogResponse, SignalResponse


def list_dashboard_signals(
    db: Session,
    *,
    limit: int,
    offset: int,
    status: str | None,
    group_id: str | None,
) -> list[dict[str, object]]:
    rows = DashboardRepository(db).signals(limit=limit, offset=offset, status=status, group_id=group_id)
    items = []
    for signal, ticket, profit in rows:
        item = SignalResponse.model_validate(signal).model_dump()
        item["created_at"] = wib_iso(signal.created_at)
        item.update({"ticket": ticket, "profit": profit})
        items.append(item)
    return items


def list_dashboard_logs(
    db: Session,
    *,
    limit: int,
    offset: int,
    level: str | None,
    search: str | None,
) -> list[dict[str, object]]:
    return [
        {**LogResponse.model_validate(row).model_dump(), "created_at": wib_iso(row.created_at)}
        for row in DashboardRepository(db).logs(level=level, search=search, limit=limit, offset=offset)
    ]


def _csv_text(value: str | None) -> str:
    text = value or ""
    if text.lstrip(" \t\r\n")[:1] in {"=", "+", "-", "@"}:
        return "'" + text
    return text


def export_dashboard_logs(db: Session, *, level: str | None, search: str | None) -> str:
    rows = DashboardRepository(db).logs(level=level, search=search, limit=10000)
    output = io.StringIO(newline="")
    writer = csv.writer(output)
    writer.writerow(["id", "created_at", "level", "source", "message"])
    for row in rows:
        writer.writerow(
            [row.id, wib_iso(row.created_at), _csv_text(row.level), _csv_text(row.source), _csv_text(row.message)]
        )
    return output.getvalue()


def _trade_dict(trade: Trade, group_id: str | None, group_name: str | None) -> dict[str, object]:
    return {
        "id": trade.id,
        "signal_id": trade.signal_id,
        "ticket": trade.ticket,
        "group_id": group_id,
        "group_name": group_name,
        "symbol": trade.symbol,
        "action": trade.action,
        "lots": trade.lots,
        "entry": trade.entry,
        "sl": trade.sl,
        "tp": trade.tp,
        "exec_price": trade.exec_price,
        "profit": trade.profit,
        "result": trade.result,
        "opened_at": wib_iso(trade.opened_at),
        "closed_at": wib_iso(trade.closed_at),
    }


def _trade_query_dates(
    db: Session,
    date_from: datetime | None,
    date_to: datetime | None,
) -> tuple[datetime | None, datetime | None]:
    start = wib_datetime_to_utc_naive(date_from) if date_from else None
    end = wib_datetime_to_utc_naive(date_to) if date_to else None
    if start and end and start > end:
        raise ValueError("date_from harus sebelum atau sama dengan date_to")
    return start, end


def _trade_rows(
    db: Session,
    *,
    date_from: datetime | None,
    date_to: datetime | None,
    group_id: str | None,
    symbol: str | None,
    result: str | None,
    limit: int,
    offset: int = 0,
) -> list[tuple[Trade, str | None, str | None]]:
    start, end = _trade_query_dates(db, date_from, date_to)
    return DashboardRepository(db).trades(
        date_from=start,
        date_to=end,
        group_id=group_id,
        symbol=symbol,
        result=result,
        limit=limit,
        offset=offset,
    )


def list_dashboard_trades(
    db: Session,
    *,
    date_from: datetime | None,
    date_to: datetime | None,
    group_id: str | None,
    symbol: str | None,
    result: str | None,
    limit: int,
    offset: int,
) -> list[dict[str, object]]:
    rows = _trade_rows(
        db,
        date_from=date_from,
        date_to=date_to,
        group_id=group_id,
        symbol=symbol,
        result=result,
        limit=limit,
        offset=offset,
    )
    return [_trade_dict(trade, row_group_id, group_name) for trade, row_group_id, group_name in rows]


def export_dashboard_trades(
    db: Session,
    *,
    date_from: datetime | None,
    date_to: datetime | None,
    group_id: str | None,
    symbol: str | None,
    result: str | None,
) -> str:
    rows = _trade_rows(
        db,
        date_from=date_from,
        date_to=date_to,
        group_id=group_id,
        symbol=symbol,
        result=result,
        limit=10000,
    )
    output = io.StringIO(newline="")
    writer = csv.writer(output)
    writer.writerow(
        [
            "ticket",
            "group",
            "symbol",
            "action",
            "lots",
            "entry",
            "exec_price",
            "profit",
            "result",
            "opened_at",
            "closed_at",
        ]
    )
    for trade, _, group_name in rows:
        writer.writerow(
            [
                _csv_text(trade.ticket),
                _csv_text(group_name),
                _csv_text(trade.symbol),
                _csv_text(trade.action),
                trade.lots,
                trade.entry,
                trade.exec_price,
                trade.profit,
                trade.result,
                wib_iso(trade.opened_at) or "",
                wib_iso(trade.closed_at) or "",
            ]
        )
    return output.getvalue()
