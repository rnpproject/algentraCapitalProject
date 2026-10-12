"""Read-only queries used by the dashboard statistics services."""

from sqlalchemy import func, select
from sqlalchemy.orm import Session

from ..models import ClientUser, Signal, Trade


class StatisticsRepository:
    def __init__(self, session: Session) -> None:
        self._session = session

    def dashboard_data(self) -> tuple[list[str], list[tuple[str | None, float | None]], int]:
        statuses = list(self._session.scalars(select(Signal.status)))
        trade_rows = list(self._session.execute(select(Trade.result, Trade.profit)).all())
        active_clients = int(self._session.scalar(select(func.count(ClientUser.id))) or 0)
        return statuses, trade_rows, active_clients

    def group_signal_counts(self) -> list[tuple[str | None, str | None, int]]:
        rows = self._session.execute(
            select(Signal.group_id, Signal.group_name, func.count(Signal.id)).group_by(
                Signal.group_id, Signal.group_name
            )
        ).all()
        return [(group_id, group_name, int(total)) for group_id, group_name, total in rows]

    def group_trade_results(self) -> list[tuple[str | None, str | None, float | None]]:
        return list(
            self._session.execute(
                select(Signal.group_id, Trade.result, Trade.profit).join(Trade, Trade.signal_id == Signal.id)
            ).all()
        )
