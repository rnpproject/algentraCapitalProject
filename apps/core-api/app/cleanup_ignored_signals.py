"""Hapus pesan Telegram non-sinyal (status IGNORED) dan lognya dari database.

Dry-run secara default; tambahkan --apply untuk benar-benar menghapus.
Baris yang sudah terhubung ke trade atau eksekusi EA tidak pernah dihapus.
"""

import argparse

from sqlalchemy import delete, exists, func, select

from .database import SessionLocal
from .models import EAExecution, Signal, SystemLog, Trade


def cleanup(apply: bool) -> dict[str, int]:
    linked = exists().where(Trade.signal_id == Signal.id) | exists().where(EAExecution.signal_id == Signal.id)
    signal_filter = (Signal.status == "IGNORED") & ~linked
    log_filter = (SystemLog.source == "Signal") & SystemLog.message.like("%: IGNORED %")
    with SessionLocal() as db:
        counts = {
            "signals": db.scalar(select(func.count()).select_from(Signal).where(signal_filter)) or 0,
            "logs": db.scalar(select(func.count()).select_from(SystemLog).where(log_filter)) or 0,
        }
        if apply:
            db.execute(delete(Signal).where(signal_filter).execution_options(synchronize_session=False))
            db.execute(delete(SystemLog).where(log_filter).execution_options(synchronize_session=False))
            db.commit()
    return counts


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--apply", action="store_true", help="hapus data (default: hanya hitung)")
    args = parser.parse_args()
    result = cleanup(args.apply)
    print(
        f"{'Dihapus' if args.apply else 'Akan dihapus (dry-run)'}: {result['signals']} sinyal IGNORED, {result['logs']} log"
    )
