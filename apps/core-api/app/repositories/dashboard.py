"""Database reads for the admin dashboard endpoints."""

from datetime import datetime, timezone

from sqlalchemy import select
from sqlalchemy.orm import Session

from ..models import Signal, SystemLog, Trade


class DashboardRepository:
    def __init__(self, session: Session) -> None:
        self._session = session

    @property
    def is_sqlite(self) -> bool:
        return self._session.get_bind().dialect.name == "sqlite"

    def signals(
        self,
        *,
        limit: int,
        offset: int,
        status: str | None,
        group_id: str | None,
    ) -> list[tuple[Signal, str | None, float | None]]:
        query = select(Signal, Trade.ticket, Trade.profit).outerjoin(Trade, Trade.signal_id == Signal.id)
        if status:
            query = query.where(Signal.status == status.upper())
        if group_id:
            query = query.where(Signal.group_id == group_id)
        query = query.order_by(Signal.created_at.desc(), Signal.id.desc()).offset(offset).limit(limit)
        return list(self._session.execute(query).all())

    def logs(self, *, level: str | None, search: str | None, limit: int, offset: int = 0) -> list[SystemLog]:
        query = select(SystemLog)
        if level:
            query = query.where(SystemLog.level == level.upper())
        if search:
            query = query.where(SystemLog.message.contains(search, autoescape=True))
        query = query.order_by(SystemLog.created_at.desc(), SystemLog.id.desc()).offset(offset).limit(limit)
        return list(self._session.scalars(query))

    def trades(
        self,
        *,
        date_from: datetime | None,
        date_to: datetime | None,
        group_id: str | None,
        symbol: str | None,
        result: str | None,
        limit: int,
        offset: int = 0,
    ) -> list[tuple[Trade, str | None, str | None]]:
        if not self.is_sqlite:
            date_from = date_from.replace(tzinfo=timezone.utc) if date_from else None
            date_to = date_to.replace(tzinfo=timezone.utc) if date_to else None
        query = select(Trade, Signal.group_id, Signal.group_name).outerjoin(Signal, Signal.id == Trade.signal_id)
        if date_from:
            query = query.where(Trade.opened_at >= date_from)
        if date_to:
            query = query.where(Trade.opened_at <= date_to)
        if group_id:
            query = query.where(Signal.group_id == group_id)
        if symbol:
            query = query.where(Trade.symbol == symbol.upper())
        if result:
            query = query.where(Trade.result == result.upper())
        query = query.order_by(Trade.opened_at.desc(), Trade.id.desc()).offset(offset).limit(limit)
        return list(self._session.execute(query).all())
