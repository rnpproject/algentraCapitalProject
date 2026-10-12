"""Import SQLite data into Supabase development while redacting legacy EA token ciphertext."""

import argparse
from collections.abc import Iterator
from datetime import date, datetime, timezone
import hashlib
import json
from pathlib import Path

from sqlalchemy import (
    Boolean,
    Date,
    DateTime,
    Float,
    Integer,
    MetaData,
    Table,
    create_engine,
    func,
    inspect,
    select,
    text,
)
from sqlalchemy.dialects.postgresql import insert as pg_insert
from sqlalchemy.engine import URL
from sqlalchemy.sql.schema import Column

from . import models  # noqa: F401 - register the complete SQLAlchemy metadata
from .core.config import PROJECT_ROOT
from .db_base import Base, EXPECTED_SCHEMA_REVISION
from .migration_config import load_development_database_config


# Legacy encrypted account tokens are never imported into Supabase; the
# corresponding token_hash remains sufficient to authenticate existing EAs.
REDACTED_COLUMNS = {"mt5_accounts": frozenset({"token_ciphertext"})}


def _data_columns(table: Table) -> list[Column]:
    redacted = REDACTED_COLUMNS.get(table.name, frozenset())
    return [column for column in table.columns if column.name not in redacted]


def _batches(rows, batch_size: int = 500) -> Iterator[list[dict]]:
    batch: list[dict] = []
    for row in rows:
        batch.append(dict(row))
        if len(batch) >= batch_size:
            yield batch
            batch = []
    if batch:
        yield batch


def _validate_target(connection) -> None:
    database_config = load_development_database_config()
    marker = connection.execute(
        text("SELECT shobj_description(oid, 'pg_database') FROM pg_database WHERE datname = current_database()")
    ).scalar_one_or_none()
    if marker != f"algentra:development:{database_config.project_ref}":
        raise RuntimeError("Import data ditolak: target bukan project development yang ditandai dengan benar")

    revision = connection.execute(text("SELECT version_num FROM public.alembic_version")).scalar_one_or_none()
    if revision != EXPECTED_SCHEMA_REVISION:
        raise RuntimeError(
            "Import data ditolak: jalankan migrasi development terlebih dahulu "
            f"(expected={EXPECTED_SCHEMA_REVISION}, applied={revision!r})"
        )
    schema_marker = connection.execute(
        text("SELECT obj_description('app'::regnamespace, 'pg_namespace')")
    ).scalar_one_or_none()
    if schema_marker != f"algentra:revision:{EXPECTED_SCHEMA_REVISION}":
        raise RuntimeError("Import data ditolak: comment revision schema app tidak cocok dengan Alembic")


def _canonical_value(value, column: Column):
    if value is None:
        return None
    if isinstance(column.type, Boolean):
        return bool(value)
    if isinstance(column.type, DateTime):
        if isinstance(value, str):
            value = datetime.fromisoformat(value)
        if value.tzinfo is None:
            value = value.replace(tzinfo=timezone.utc)
        else:
            value = value.astimezone(timezone.utc)
        return value.isoformat(timespec="microseconds")
    if isinstance(column.type, Date):
        if isinstance(value, str):
            value = date.fromisoformat(value)
        return value.isoformat()
    if isinstance(column.type, Integer):
        return int(value)
    if isinstance(column.type, Float):
        return float(value)
    if isinstance(value, bytes):
        return {"bytes_hex": value.hex()}
    return value


def _table_checksum(connection, table: Table, source_table: Table | None = None) -> tuple[int, str]:
    source = source_table if source_table is not None else table
    target_columns = _data_columns(table)
    order_columns = list(table.primary_key.columns) or target_columns
    statement = select(*(source.c[column.name] for column in target_columns)).order_by(
        *(source.c[column.name] for column in order_columns)
    )
    digest = hashlib.sha256()
    row_count = 0
    for row in connection.execute(statement.execution_options(stream_results=True)).mappings():
        canonical = [_canonical_value(row[column.name], column) for column in target_columns]
        encoded = json.dumps(canonical, ensure_ascii=False, sort_keys=True, separators=(",", ":"), allow_nan=False)
        digest.update(encoded.encode("utf-8"))
        digest.update(b"\n")
        row_count += 1
    return row_count, digest.hexdigest()


def _sync_integer_primary_key_sequences(connection, table: Table) -> None:
    if len(table.primary_key.columns) != 1:
        return
    column: Column = next(iter(table.primary_key.columns))
    if not column.primary_key or column.autoincrement not in {True, "auto"}:
        return
    if column.type.python_type is not int:
        return

    sequence_name = connection.execute(
        text("SELECT pg_get_serial_sequence(:table_name, :column_name)"),
        {"table_name": f"app.{table.name}", "column_name": column.name},
    ).scalar_one_or_none()
    if sequence_name:
        last_id = connection.execute(select(func.max(column)).select_from(table)).scalar_one()
        if last_id is not None:
            connection.execute(
                text("SELECT setval(CAST(:sequence_name AS regclass), :last_id, TRUE)"),
                {"sequence_name": sequence_name, "last_id": last_id},
            )


def import_sqlite_to_supabase(source_path: Path, *, dry_run: bool = False) -> tuple[dict[str, int], dict[str, str]]:
    database_config = load_development_database_config()

    source_path = source_path.expanduser().resolve()
    if not source_path.is_file():
        raise FileNotFoundError(f"File SQLite tidak ditemukan: {source_path}")
    source_engine = create_engine(URL.create("sqlite", database=source_path.as_posix()))
    target_engine = create_engine(
        database_config.runtime_url,
        connect_args={"connect_timeout": 10},
        pool_pre_ping=True,
    )
    validation_engine = create_engine(
        database_config.migration_url,
        connect_args={"connect_timeout": 10},
        pool_pre_ping=True,
    )
    expected_tables = list(Base.metadata.sorted_tables)
    counts: dict[str, int] = {}
    actions: dict[str, str] = {}
    try:
        source_table_names = set(inspect(source_engine).get_table_names())
        expected_names = {table.name for table in expected_tables}
        missing_tables = sorted(expected_names - source_table_names)
        unexpected_tables = sorted(source_table_names - expected_names)
        if missing_tables or unexpected_tables:
            raise RuntimeError(
                "Skema SQLite tidak sama dengan model rilis ini. "
                f"Tabel hilang={missing_tables}; tabel tambahan={unexpected_tables}. "
                "Selesaikan pemetaan versi skema sebelum impor."
            )

        source_metadata = MetaData()
        source_tables = {
            name: Table(name, source_metadata, autoload_with=source_engine) for name in sorted(expected_names)
        }
        for target_table in expected_tables:
            source_table = source_tables[target_table.name]
            source_columns = {column.name for column in source_table.columns}
            target_columns = {column.name for column in target_table.columns}
            missing_columns = sorted(target_columns - source_columns)
            legacy_columns = REDACTED_COLUMNS.get(target_table.name, frozenset())
            extra_columns = sorted((source_columns - target_columns) - legacy_columns)
            if missing_columns or extra_columns:
                raise RuntimeError(
                    f"Kolom SQLite tidak cocok pada {target_table.name}: "
                    f"kolom hilang={missing_columns}; kolom lama/tidak dikenal={extra_columns}"
                )

        # The runtime role intentionally cannot read public.alembic_version.
        # Validate the migration marker with the development-only migrator,
        # then perform all data reads/writes through the least-privilege API role.
        with validation_engine.connect() as validation_connection:
            _validate_target(validation_connection)

        with target_engine.begin() as target_connection:
            target_connection.exec_driver_sql("SET search_path TO app")
            with source_engine.connect() as source_connection:
                for target_table in expected_tables:
                    source_table = source_tables[target_table.name]
                    source_count, source_checksum = _table_checksum(source_connection, target_table, source_table)
                    target_count, target_checksum = _table_checksum(target_connection, target_table)
                    counts[target_table.name] = source_count
                    if source_count == target_count and source_checksum == target_checksum:
                        actions[target_table.name] = "already identical"
                        continue
                    if dry_run:
                        actions[target_table.name] = (
                            f"would import/verify ({target_count} existing, {source_count} source)"
                        )
                        continue

                    result = source_connection.execute(
                        select(*(source_table.c[column.name] for column in _data_columns(target_table))).order_by(
                            *(
                                source_table.c[column.name]
                                for column in (list(target_table.primary_key.columns) or _data_columns(target_table))
                            )
                        )
                    )
                    for batch in _batches(result.mappings()):
                        for row in batch:
                            for column in target_table.columns:
                                if isinstance(column.type, Boolean) and row.get(column.name) is not None:
                                    row[column.name] = bool(row[column.name])
                        target_connection.execute(pg_insert(target_table).on_conflict_do_nothing(), batch)
                    verified_count, verified_checksum = _table_checksum(target_connection, target_table)
                    if verified_count != source_count or verified_checksum != source_checksum:
                        raise RuntimeError(
                            f"Checksum berbeda setelah impor {target_table.name}; "
                            "transaksi dibatalkan agar data tidak tertimpa atau terduplikasi"
                        )
                    actions[target_table.name] = "imported and verified"

            if not dry_run:
                for target_table in expected_tables:
                    _sync_integer_primary_key_sequences(target_connection, target_table)
    finally:
        source_engine.dispose()
        target_engine.dispose()
        validation_engine.dispose()

    return counts, actions


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "source",
        nargs="?",
        type=Path,
        default=PROJECT_ROOT / "backend" / "data" / "app.db",
        help="Source SQLite file (default: backend/data/app.db)",
    )
    parser.add_argument(
        "--dry-run", action="store_true", help="Validate project/schema and report checksums without writing"
    )
    args = parser.parse_args()
    counts, actions = import_sqlite_to_supabase(args.source, dry_run=args.dry_run)
    total = sum(counts.values())
    title = "Dry-run checked" if args.dry_run else "Imported/verified"
    print(f"{title} {total} source rows across {len(counts)} tables in Supabase development:")
    for table_name, row_count in counts.items():
        print(f"  {table_name}: {row_count} source rows; {actions[table_name]}")


if __name__ == "__main__":
    main()
