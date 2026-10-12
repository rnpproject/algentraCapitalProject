"""Database reads needed to build the public performance view."""

from datetime import datetime

from sqlalchemy import func, select
from sqlalchemy.orm import Session

from ..models import (
    ClientUser,
    MasterCopyState,
    MT5MasterAccountState,
    MT5MasterEquityCandle,
    MT5MasterMarketState,
)


class PublicPerformanceRepository:
    def __init__(self, session: Session) -> None:
        self._session = session

    def registered_client_count(self) -> int:
        return int(self._session.execute(select(func.count(ClientUser.id))).scalar_one())

    def master_account(self) -> MT5MasterAccountState | None:
        return self._session.get(MT5MasterAccountState, 1)

    def master_copy_state(self) -> MasterCopyState | None:
        return self._session.get(MasterCopyState, 1)

    def market_state(self) -> MT5MasterMarketState | None:
        return self._session.get(MT5MasterMarketState, 1)

    def latest_equity_account_key(self) -> str | None:
        return self._session.execute(
            select(MT5MasterEquityCandle.account_key).order_by(MT5MasterEquityCandle.observed_at.desc()).limit(1)
        ).scalar_one_or_none()

    def equity_candles(self, account_key: str, session_start: datetime) -> list[MT5MasterEquityCandle]:
        return list(
            self._session.scalars(
                select(MT5MasterEquityCandle)
                .where(
                    MT5MasterEquityCandle.account_key == account_key,
                    MT5MasterEquityCandle.minute_start >= session_start,
                )
                .order_by(MT5MasterEquityCandle.minute_start.asc())
            )
        )
