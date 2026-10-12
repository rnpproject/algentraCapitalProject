"""Persistent risk metrics used by the trading controls service."""

from datetime import datetime, timezone

from sqlalchemy import func, select
from sqlalchemy.orm import Session

from ..models import Trade


class RiskRepository:
    def __init__(self, session: Session) -> None:
        self._session = session

    def daily_profit_and_open_trade_count(self, start_of_day: datetime) -> tuple[float, int]:
        if self._session.get_bind().dialect.name != "sqlite":
            start_of_day = start_of_day.replace(tzinfo=timezone.utc)
        daily_profit = float(
            self._session.scalar(
                select(func.coalesce(func.sum(Trade.profit), 0.0)).where(
                    Trade.closed_at.is_not(None),
                    Trade.closed_at >= start_of_day,
                )
            )
            or 0.0
        )
        open_trades = int(
            self._session.scalar(
                select(func.count(Trade.id)).where(
                    Trade.opened_at.is_not(None),
                    Trade.closed_at.is_(None),
                )
            )
            or 0
        )
        return daily_profit, open_trades
