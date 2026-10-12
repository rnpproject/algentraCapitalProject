"""Process-local view of the most recent EA heartbeat."""

from dataclasses import dataclass
from datetime import datetime
from threading import RLock


@dataclass(frozen=True, slots=True)
class EAHeartbeat:
    active: bool
    last_heartbeat: datetime
    terminal: str | None
    version: str | None
    symbol: str | None


class EAHeartbeatStore:
    def __init__(self) -> None:
        self._lock = RLock()
        self._heartbeat: EAHeartbeat | None = None

    def current(self) -> EAHeartbeat | None:
        with self._lock:
            return self._heartbeat

    def publish(self, heartbeat: EAHeartbeat) -> EAHeartbeat:
        with self._lock:
            self._heartbeat = heartbeat
            return heartbeat

    def clear(self) -> None:
        with self._lock:
            self._heartbeat = None


ea_heartbeat_state = EAHeartbeatStore()
