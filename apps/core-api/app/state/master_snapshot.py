"""Thread-safe, process-local copy of the latest master terminal snapshot."""

from dataclasses import dataclass
from datetime import datetime, timedelta
from threading import RLock
from typing import Iterable


@dataclass(frozen=True, slots=True)
class MasterPosition:
    ticket: str
    symbol: str
    action: str
    lots: float
    entry_price: float
    sl: float | None
    tp: float | None


@dataclass(frozen=True, slots=True)
class MasterSnapshot:
    observed_at: datetime
    positions: tuple[MasterPosition, ...]
    revision: int
    account: "MasterAccountSnapshot | None"
    market_quotes_json: str | None
    market_observed_at: datetime | None


@dataclass(frozen=True, slots=True)
class MasterAccountSnapshot:
    balance: float
    equity: float
    floating_profit: float
    currency: str
    trade_mode: str
    observed_at: datetime


class MasterSnapshotStore:
    """Keep the replaceable current snapshot in memory between EA reports."""

    def __init__(self) -> None:
        self._lock = RLock()
        self._snapshot: MasterSnapshot | None = None
        self._revision = 0

    def current(self) -> MasterSnapshot | None:
        with self._lock:
            return self._snapshot

    def clear(self) -> None:
        with self._lock:
            self._snapshot = None
            self._revision = 0

    def publish(
        self,
        positions: Iterable[MasterPosition],
        observed_at: datetime,
        *,
        account: MasterAccountSnapshot | None = None,
        market_quotes_json: str | None = None,
    ) -> MasterSnapshot:
        immutable_positions = tuple(positions)
        with self._lock:
            previous = self._snapshot
            previous_positions = previous.positions if previous is not None else None
            if immutable_positions != previous_positions:
                self._revision += 1
            self._snapshot = MasterSnapshot(
                observed_at=observed_at,
                positions=immutable_positions,
                revision=self._revision,
                account=account if account is not None else (previous.account if previous is not None else None),
                market_quotes_json=(
                    market_quotes_json
                    if market_quotes_json is not None
                    else (previous.market_quotes_json if previous is not None else None)
                ),
                market_observed_at=(
                    observed_at
                    if market_quotes_json is not None
                    else (previous.market_observed_at if previous is not None else None)
                ),
            )
            return self._snapshot

    def is_online(self, now: datetime, max_age: timedelta) -> bool:
        snapshot = self.current()
        return bool(snapshot and now >= snapshot.observed_at and now - snapshot.observed_at <= max_age)


master_snapshot_state = MasterSnapshotStore()
