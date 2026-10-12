"""Read-only persistence queries for the MT5 performance view."""

from sqlalchemy import select
from sqlalchemy.orm import Session

from ..models import MT5Account, MT5AccountState, MT5HistoryDeal


class MT5PerformanceRepository:
    def __init__(self, session: Session) -> None:
        self._session = session

    def account_states(self, account_ids: list[int]) -> list[tuple[MT5Account, MT5AccountState | None]]:
        return list(
            self._session.execute(
                select(MT5Account, MT5AccountState)
                .outerjoin(MT5AccountState, MT5AccountState.account_id == MT5Account.id)
                .where(MT5Account.id.in_(account_ids))
            ).all()
        )

    def history_deals(self, account_ids: list[int]) -> list[MT5HistoryDeal]:
        return list(
            self._session.scalars(
                select(MT5HistoryDeal)
                .where(MT5HistoryDeal.account_id.in_(account_ids))
                .order_by(MT5HistoryDeal.deal_time_msc.asc(), MT5HistoryDeal.id.asc())
            )
        )
