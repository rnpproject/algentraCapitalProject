"""Small transaction primitives shared by database repositories."""

import hashlib

from sqlalchemy import text
from sqlalchemy.orm import Session


def begin_serialized_write(session: Session, lock_key: str) -> None:
    """Serialize one logical write key across SQLite and PostgreSQL.

    SQLite uses its database write lock. PostgreSQL uses a transaction-scoped
    advisory lock, which also protects keys whose target row does not exist yet.
    """
    if session.get_bind().dialect.name == "sqlite":
        session.execute(text("BEGIN IMMEDIATE"))
        return
    lock_id = int.from_bytes(hashlib.sha256(lock_key.encode("utf-8")).digest()[:8], "big", signed=True)
    session.execute(text("SELECT pg_advisory_xact_lock(:lock_id)"), {"lock_id": lock_id})
