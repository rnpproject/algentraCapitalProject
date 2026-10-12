from alembic import context
from sqlalchemy import create_engine, text
from sqlalchemy.pool import NullPool

from app import models  # noqa: F401
from app.db_base import Base
from app.migration_config import load_development_database_config


config = context.config
target_metadata = Base.metadata


def _migration_url() -> str:
    return load_development_database_config().migration_url


def _assert_development_target(connection) -> None:
    database_config = load_development_database_config()
    marker = connection.execute(
        text(
            "SELECT shobj_description(oid, 'pg_database') FROM pg_database "
            "WHERE datname = current_database()"
        )
    ).scalar_one_or_none()
    if marker != f"algentra:development:{database_config.project_ref}":
        raise RuntimeError(
            "Migrasi ditolak: target PostgreSQL tidak memiliki penanda project development yang cocok "
            "dengan SUPABASE_DEV_PROJECT_REF"
        )


def run_migrations_offline() -> None:
    raise RuntimeError("Migrasi offline dilarang karena target development harus diverifikasi langsung")


def run_migrations_online() -> None:
    connectable = create_engine(
        _migration_url(),
        connect_args={"connect_timeout": 10},
        poolclass=NullPool,
        pool_pre_ping=True,
    )
    try:
        with connectable.connect() as connection:
            _assert_development_target(connection)
            connection.exec_driver_sql("SET search_path TO app")
            connection.commit()
            context.configure(
                connection=connection,
                target_metadata=target_metadata,
                version_table_schema="public",
                compare_type=True,
            )
            with context.begin_transaction():
                context.run_migrations()
    finally:
        connectable.dispose()


if context.is_offline_mode():
    run_migrations_offline()
else:
    run_migrations_online()
