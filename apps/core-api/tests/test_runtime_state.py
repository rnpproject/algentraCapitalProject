from datetime import datetime, timedelta, timezone

from app.state import EAHeartbeat, MasterAccountSnapshot, MasterPosition
from app.state.ea_heartbeat import EAHeartbeatStore
from app.state.master_snapshot import MasterSnapshotStore


def test_master_snapshot_revisions_only_change_when_positions_change():
    store = MasterSnapshotStore()
    now = datetime.now(timezone.utc)
    position = MasterPosition("123", "EURUSD", "BUY", 0.1, 1.25, None, None)

    first = store.publish([position], now)
    second = store.publish([position], now + timedelta(seconds=1))

    assert first.revision == 1
    assert second.revision == first.revision
    assert second.observed_at > first.observed_at

    third = store.publish([], now + timedelta(seconds=2))
    assert third.revision == second.revision + 1


def test_master_snapshot_keeps_latest_optional_telemetry():
    store = MasterSnapshotStore()
    now = datetime.now(timezone.utc)
    account = MasterAccountSnapshot(1000, 1010, 10, "USD", "REAL", now)
    first = store.publish([], now, account=account, market_quotes_json='[{"symbol":"EURUSD"}]')
    second = store.publish([], now + timedelta(seconds=1))

    assert second.account == account
    assert second.market_quotes_json == first.market_quotes_json
    assert second.market_observed_at == now


def test_master_snapshot_freshness_and_clear():
    store = MasterSnapshotStore()
    now = datetime.now(timezone.utc)
    store.publish([], now)

    assert store.is_online(now + timedelta(seconds=20), timedelta(seconds=20))
    assert not store.is_online(now + timedelta(seconds=21), timedelta(seconds=20))

    store.clear()
    assert store.current() is None
    assert not store.is_online(now, timedelta(seconds=20))


def test_ea_heartbeat_store_replaces_state_and_clears():
    store = EAHeartbeatStore()
    first = EAHeartbeat(True, datetime.now(timezone.utc), "terminal-1", "1.0", "XAUUSD")
    second = EAHeartbeat(False, datetime.now(timezone.utc), "terminal-1", "1.0", "XAUUSD")

    assert store.publish(first) == first
    assert store.current() == first
    assert store.publish(second) == second
    assert store.current() == second

    store.clear()
    assert store.current() is None
