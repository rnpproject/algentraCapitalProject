from collections import Counter
from collections.abc import Iterable
from typing import Any

from datetime import datetime, time, timedelta, timezone

from sqlalchemy.orm import Session

from ..repositories.statistics import StatisticsRepository
from ..core.time_utils import as_utc, wib_iso
from ..core.time_utils import wib_datetime_to_utc_naive
from ..state import ea_heartbeat_state


def calculate_trade_stats(trades: Iterable[Any]) -> dict[str, int | float]:
    wins = 0
    losses = 0
    total_profit = 0.0
    for trade in trades:
        result = (getattr(trade, "result", None) or "").upper()
        profit = getattr(trade, "profit", 0.0)
        total_profit += float(profit or 0.0)
        if result == "WIN":
            wins += 1
        elif result == "LOSS":
            losses += 1

    decided_trades = wins + losses
    winrate = (wins / decided_trades * 100.0) if decided_trades else 0.0
    return {
        "wins": wins,
        "losses": losses,
        "winrate": round(winrate, 2),
        "total_profit": round(total_profit, 2),
    }


def calculate_daily_loss(trades: Iterable[Any], *, day_start_utc: datetime) -> float:
    loss = 0.0
    for trade in trades:
        closed_at = getattr(trade, "closed_at", None)
        if closed_at is None:
            continue
        if as_utc(closed_at).replace(tzinfo=None) >= day_start_utc:
            loss += min(0.0, float(getattr(trade, "profit", 0.0) or 0.0))
    return round(abs(loss), 2)


def get_dashboard_stats(db: Session) -> dict[str, int | float]:
    signal_rows, trades, active_clients, unknown_signal_statuses = StatisticsRepository(db).dashboard_data()
    status_counts = Counter(status.upper() for status in signal_rows)
    trade_stats = calculate_trade_stats(trades)
    ea = ea_heartbeat_state.current()
    heartbeat = ea.last_heartbeat if ea else None
    if heartbeat is not None:
        heartbeat = as_utc(heartbeat)
        ea_online = bool(ea.active and (datetime.now(timezone.utc) - heartbeat).total_seconds() <= 90)
    else:
        ea_online = False
    now_utc = datetime.now(timezone.utc)
    today_wib = now_utc.astimezone(timezone(timedelta(hours=7))).date()
    day_start_utc = wib_datetime_to_utc_naive(datetime.combine(today_wib, time.min))
    daily_loss = calculate_daily_loss(trades, day_start_utc=day_start_utc)
    return {
        "signals_total": sum(status_counts.values()),
        # WIN/LOSS/BE are terminal states for signals that reached execution.
        "signals_executed": status_counts["EXECUTED"]
        + status_counts["WIN"]
        + status_counts["LOSS"]
        + status_counts["BE"],
        "signals_pending": status_counts["PENDING"] + status_counts["CLAIMED"],
        "signals_processing": status_counts["PROCESSING"],
        "signals_rejected": status_counts["REJECTED"] + status_counts["FAILED"],
        "signals_ignored": status_counts["IGNORED"],
        "signals_expired": status_counts["EXPIRED"],
        "signal_status_unknown": bool(unknown_signal_statuses),
        "signal_status_unknown_values": unknown_signal_statuses,
        "active_clients": active_clients,
        "ea_online": ea_online,
        "ea_last_heartbeat": wib_iso(heartbeat),
        "daily_loss": daily_loss,
        **trade_stats,
    }


def get_group_stats(db: Session) -> list[dict[str, int | float | str | None]]:
    repository = StatisticsRepository(db)
    signal_counts = repository.group_signal_counts()
    trades = repository.group_trade_results()
    grouped: dict[str | None, dict[str, float | int]] = {}
    for group_id, result, profit in trades:
        stats = grouped.setdefault(group_id, {"wins": 0, "losses": 0, "profit": 0.0})
        result = (result or "").upper()
        if result == "WIN":
            stats["wins"] += 1
        elif result == "LOSS":
            stats["losses"] += 1
        stats["profit"] += float(profit or 0.0)

    output = []
    for group_id, group_name, total in signal_counts:
        stats = grouped.get(group_id, {"wins": 0, "losses": 0, "profit": 0.0})
        decided = int(stats["wins"]) + int(stats["losses"])
        output.append(
            {
                "group_id": group_id,
                "group_name": group_name or "Unknown",
                "signals_total": total,
                "wins": int(stats["wins"]),
                "losses": int(stats["losses"]),
                "winrate": round(int(stats["wins"]) / decided * 100.0, 2) if decided else 0.0,
                "profit": round(float(stats["profit"]), 2),
            }
        )
    return sorted(output, key=lambda row: row["signals_total"], reverse=True)
