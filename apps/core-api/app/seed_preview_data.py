"""Seed isolated local dashboard preview records. Never use against production."""

from datetime import timedelta
import os

from sqlalchemy import delete, select

from .database import SessionLocal, engine
from .models import ClientUser, Signal, SystemLog, Trade, utc_now
from .state import EAHeartbeat, ea_heartbeat_state, master_snapshot_state
from .state.master_snapshot import MasterAccountSnapshot, MasterPosition


def main() -> None:
    if engine.dialect.name != "sqlite":
        raise RuntimeError("Dummy dashboard data may only be seeded into SQLite")

    now = utc_now()
    signal_specs = [
        *(('EXECUTED', f'XAUUSD BUY 2325 SL 2318 TP 2335 #{i}', f'BUY XAUUSD entry 2325 #{i}') for i in range(1, 5)),
        *(('REJECTED', f'XAUUSD SELL 2340 SL 2348 TP 2328 #{i}', f'SELL XAUUSD rejected #{i}') for i in range(1, 30)),
        *(('PROCESSING', f'XAUUSD BUY 2325 SL 2318 TP 2335 #{i}', f'XAUUSD order processing #{i}') for i in range(1, 14)),
    ]
    trade_specs = [
        ("WIN", 1520.00, "731001", "BUY"), ("WIN", 1385.50, "731002", "SELL"),
        ("WIN", 1273.35, "731003", "BUY"), ("WIN", 1100.00, "731004", "BUY"),
        ("WIN", -500.00, "731005", "SELL"), ("WIN", 700.00, "731006", "BUY"),
        ("WIN", 600.00, "731007", "SELL"), ("LOSS", -250.00, "731008", "SELL"),
        ("LOSS", -150.00, "731009", "BUY"), ("LOSS", -100.00, "731010", "SELL"),
        ("LOSS", -100.00, "731011", "BUY"), ("LOSS", -100.00, "731012", "SELL"),
        ("LOSS", -100.00, "731013", "BUY"),
    ]

    with SessionLocal() as db:
        seeded = db.scalar(select(Signal.id).where(Signal.message_id == "preview-1").limit(1))
        if seeded is not None and os.environ.get("PREVIEW_RESET") == "1":
            preview_ids = select(Signal.id).where(Signal.message_id.like("preview-%"))
            db.execute(delete(Trade).where(Trade.signal_id.in_(preview_ids)))
            db.execute(delete(Signal).where(Signal.message_id.like("preview-%")))
            db.execute(delete(ClientUser).where(ClientUser.email.like("preview-client-%@example.test")))
            db.execute(delete(SystemLog).where(SystemLog.source == "Preview"))
            db.commit()
            seeded = None
        if seeded is None:
            signals: list[Signal] = []
            for index, (status, raw, normalized) in enumerate(signal_specs, start=1):
                signal = Signal(
                    group_id="preview-group", group_name="Demo Signals", message_id=f"preview-{index}",
                    sender_name="Preview Feed", raw_text=raw, normalized_text=normalized,
                    confidence=0.91 if status != "IGNORED" else 0.12,
                    content_hash=f"preview-content-hash-{index}", status=status,
                    created_at=now - timedelta(minutes=8 * index),
                )
                db.add(signal)
                signals.append(signal)
            db.flush()
            for index, (result, profit, ticket, action) in enumerate(trade_specs):
                db.add(Trade(
                    signal_id=signals[index % 4].id, ticket=ticket, symbol="XAUUSD", action=action,
                    lots=0.10, entry=2320 + index, exec_price=2320 + index, profit=profit, result=result,
                    opened_at=now - timedelta(days=index + 1), closed_at=now - timedelta(days=index + 1, hours=-1),
                ))
            for index in range(4):
                db.add(ClientUser(
                    email=f"preview-client-{index + 1}@example.test", password_hash="preview-only",
                    verified_at=now, created_at=now,
                ))
            db.add_all([
                SystemLog(level="INFO", source="Preview", message="Demo terminal heartbeat received", created_at=now),
                SystemLog(level="WARN", source="Preview", message="Dummy signal has PROCESSING status", created_at=now - timedelta(minutes=10)),
                SystemLog(level="ERROR", source="Preview", message="Sample dashboard error log", created_at=now - timedelta(minutes=25)),
            ])
            db.commit()

    publish_preview_runtime()


def publish_preview_runtime() -> None:
    now = utc_now()
    ea_heartbeat_state.publish(EAHeartbeat(True, now, "Preview terminal", "1.0-preview", "XAUUSD"))
    master_snapshot_state.publish(
        [MasterPosition("900001", "XAUUSD", "BUY", 0.1, 2320.0, 2312.0, 2334.0)],
        now,
        account=MasterAccountSnapshot(10250.00, 10450.00, 200.00, "USD", "demo", now),
        market_quotes_json='[{"symbol":"XAUUSD","bid":2325.1,"ask":2325.4,"time_msc":0}]',
    )


if __name__ == "__main__":
    main()
