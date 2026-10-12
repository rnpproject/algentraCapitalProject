"""Database operations for incoming and parsed trading signals."""

from sqlalchemy import select
from sqlalchemy.exc import IntegrityError
from sqlalchemy.orm import Session

from ..models import Signal, SystemLog


class SignalRepository:
    def __init__(self, session: Session) -> None:
        self._session = session

    def find_by_message(self, group_id: str, message_id: str) -> Signal | None:
        return self._session.execute(
            select(Signal).where(
                Signal.group_id == group_id,
                Signal.message_id == message_id,
            )
        ).scalar_one_or_none()

    def find_by_content_hash(self, content_hash: str) -> Signal | None:
        return self._session.execute(select(Signal).where(Signal.content_hash == content_hash)).scalar_one_or_none()

    def save_failed(self, signal: Signal, log: SystemLog) -> Signal:
        self._session.add_all([signal, log])
        self._session.commit()
        self._session.refresh(signal)
        return signal

    def save_processed(self, signal: Signal, log: SystemLog) -> tuple[Signal | None, bool]:
        """Persist a signal and report a concurrent content-hash duplicate."""
        self._session.add_all([signal, log])
        try:
            self._session.commit()
        except IntegrityError:
            self._session.rollback()
            duplicate = self.find_by_content_hash(signal.content_hash or "")
            if duplicate is not None:
                return duplicate, True
            raise

        self._session.refresh(signal)
        return signal, False
