from collections.abc import Generator
from pathlib import Path

from sqlalchemy import create_engine, inspect, text
from sqlalchemy import event
from sqlalchemy.exc import SQLAlchemyError
from sqlalchemy.orm import Session, sessionmaker
from sqlalchemy.pool import StaticPool

from .core.config import get_settings
from .db_base import Base, EXPECTED_SCHEMA_REVISION


def _make_engine():
    database_url = get_settings().resolved_database_url
    connect_args = {"check_same_thread": False} if database_url.startswith("sqlite") else {}
    is_memory_database = database_url in {"sqlite://", "sqlite:///:memory:"}
    if database_url.startswith("sqlite:///"):
        database_path = database_url.removeprefix("sqlite:///")
        if not is_memory_database and database_path != ":memory:":
            path = Path(database_path)
            if not path.is_absolute():
                path = Path.cwd() / path
            path.parent.mkdir(parents=True, exist_ok=True)
    engine_options = {"poolclass": StaticPool} if is_memory_database else {}
    if database_url.startswith("postgresql+psycopg://"):
        connect_args = {"connect_timeout": 10}
        engine_options.update({"pool_size": 5, "max_overflow": 5, "pool_timeout": 10})
    return create_engine(database_url, connect_args=connect_args, pool_pre_ping=True, **engine_options)


engine = _make_engine()
SessionLocal = sessionmaker(bind=engine, autoflush=False, expire_on_commit=False)


if engine.dialect.name == "sqlite":

    @event.listens_for(engine, "connect")
    def configure_sqlite_connection(dbapi_connection, _connection_record) -> None:
        cursor = dbapi_connection.cursor()
        cursor.execute("PRAGMA foreign_keys=ON")
        cursor.execute("PRAGMA busy_timeout=5000")
        cursor.execute("PRAGMA journal_mode=WAL")
        cursor.close()
elif engine.dialect.name == "postgresql":

    @event.listens_for(engine, "connect")
    def configure_postgresql_connection(dbapi_connection, _connection_record) -> None:
        # Supabase's PgBouncer ignores libpq startup options such as search_path.
        # Set it on each session so unqualified ORM tables resolve to app.
        cursor = dbapi_connection.cursor()
        cursor.execute("SET search_path TO app")
        cursor.close()
        dbapi_connection.commit()


def migrate_schema() -> None:
    """Apply the small additive SQLite changes needed between early milestones."""
    if engine.dialect.name != "sqlite":
        return
    inspector = inspect(engine)
    if "signals" in inspector.get_table_names():
        columns = {column["name"] for column in inspector.get_columns("signals")}
        additions = {
            "content_hash": "VARCHAR(64)",
            "claimed_at": "DATETIME",
            "sender_id": "VARCHAR(128)",
            "sender_name": "VARCHAR(255)",
        }
        missing = {name: sql_type for name, sql_type in additions.items() if name not in columns}
        if missing:
            with engine.begin() as connection:
                for name, sql_type in missing.items():
                    connection.execute(text(f"ALTER TABLE signals ADD COLUMN {name} {sql_type}"))
    if "email_verifications" in inspector.get_table_names():
        columns = {column["name"] for column in inspector.get_columns("email_verifications")}
        additions = {
            "send_count": "INTEGER NOT NULL DEFAULT 1",
            "send_window_started_at": "DATETIME",
            "locked_until": "DATETIME",
        }
        missing = {name: sql_type for name, sql_type in additions.items() if name not in columns}
        if missing:
            with engine.begin() as connection:
                for name, sql_type in missing.items():
                    connection.execute(text(f"ALTER TABLE email_verifications ADD COLUMN {name} {sql_type}"))
    if "password_reset_codes" in inspector.get_table_names():
        columns = {column["name"] for column in inspector.get_columns("password_reset_codes")}
        if "last_activity_at" not in columns:
            with engine.begin() as connection:
                connection.execute(
                    text(
                        "ALTER TABLE password_reset_codes "
                        "ADD COLUMN last_activity_at DATETIME NOT NULL "
                        "DEFAULT '1970-01-01 00:00:00'"
                    )
                )
    if "mt5_accounts" in inspector.get_table_names():
        columns = {column["name"] for column in inspector.get_columns("mt5_accounts")}
        additions = {
            "plan": "VARCHAR(16) NOT NULL DEFAULT 'ZERO'",
            "execution_mode": "VARCHAR(16) NOT NULL DEFAULT 'EA'",
        }
        missing = {name: sql_type for name, sql_type in additions.items() if name not in columns}
        if missing:
            with engine.begin() as connection:
                for name, sql_type in missing.items():
                    connection.execute(text(f"ALTER TABLE mt5_accounts ADD COLUMN {name} {sql_type}"))
    if "payment_orders" in inspector.get_table_names():
        columns = {column["name"] for column in inspector.get_columns("payment_orders")}
        if "execution_mode" not in columns:
            with engine.begin() as connection:
                connection.execute(
                    text("ALTER TABLE payment_orders ADD COLUMN execution_mode VARCHAR(16) NOT NULL DEFAULT 'EA'")
                )
    if "managed_copy_profiles" in inspector.get_table_names():
        columns = {column["name"] for column in inspector.get_columns("managed_copy_profiles")}
        additions = {
            "login_status": "VARCHAR(24) NOT NULL DEFAULT 'UNCHECKED'",
            "login_message": "VARCHAR(240)",
        }
        missing = {name: sql_type for name, sql_type in additions.items() if name not in columns}
        if missing:
            with engine.begin() as connection:
                for name, sql_type in missing.items():
                    connection.execute(text(f"ALTER TABLE managed_copy_profiles ADD COLUMN {name} {sql_type}"))


def ensure_schema_ready() -> None:
    """Refuse to start against an empty or incomplete PostgreSQL schema; never create production tables."""
    if engine.dialect.name == "sqlite":
        return
    expected_tables = set(Base.metadata.tables)
    schema_inspector = inspect(engine)
    available_tables = set(schema_inspector.get_table_names(schema="app"))
    missing_tables = sorted(expected_tables - available_tables)
    if missing_tables:
        preview = ", ".join(missing_tables[:5])
        suffix = "…" if len(missing_tables) > 5 else ""
        raise RuntimeError(
            "Database schema belum disiapkan. Jalankan migrasi Alembic hanya pada Supabase development "
            f"sebelum deploy; tabel belum ada: {preview}{suffix}"
        )
    incompatible_columns = []
    for table_name, table in Base.metadata.tables.items():
        available_columns = {column["name"] for column in schema_inspector.get_columns(table_name, schema="app")}
        missing_columns = sorted(set(table.columns.keys()) - available_columns)
        if missing_columns:
            incompatible_columns.append(f"{table_name}({', '.join(missing_columns)})")
    if incompatible_columns:
        raise RuntimeError(
            "Database schema belum cocok dengan kode aplikasi; kolom belum ada: " + ", ".join(incompatible_columns[:5])
        )
    try:
        with engine.connect() as connection:
            config = get_settings()
            deployment_environment = config.app_env
            expected_project_ref = (
                (config.supabase_dev_project_ref or "").strip()
                if deployment_environment == "development"
                else (config.supabase_project_ref or "").strip()
            )
            marker = connection.execute(
                text("SELECT shobj_description(oid, 'pg_database') FROM pg_database WHERE datname = current_database()")
            ).scalar_one_or_none()
            if marker != f"algentra:{deployment_environment}:{expected_project_ref}":
                raise RuntimeError("Database environment marker does not match APP_ENV; refusing to start")
            applied_revision = connection.execute(
                text("SELECT obj_description('app'::regnamespace, 'pg_namespace')")
            ).scalar_one_or_none()
    except SQLAlchemyError as exc:
        raise RuntimeError(
            "Database belum memiliki catatan revisi Alembic; schema harus disiapkan sebelum deploy production"
        ) from exc
    expected_schema_marker = f"algentra:revision:{EXPECTED_SCHEMA_REVISION}"
    if applied_revision != expected_schema_marker:
        raise RuntimeError(
            "Revisi schema Supabase tidak cocok dengan kode aplikasi: "
            f"expected={expected_schema_marker}, applied={applied_revision!r}"
        )


def get_db() -> Generator[Session, None, None]:
    db = SessionLocal()
    try:
        yield db
    finally:
        db.close()
