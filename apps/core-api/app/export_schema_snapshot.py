"""Export a schema-only SQL snapshot from the verified development project."""

import argparse
import os
from pathlib import Path
import shutil
import subprocess

from alembic.config import Config
from alembic.script import ScriptDirectory
from sqlalchemy import create_engine, text
from sqlalchemy.engine import make_url

from .core.config import PROJECT_ROOT
from .migration_config import load_development_database_config


def export_schema_snapshot(destination: Path) -> None:
    database_config = load_development_database_config()

    migration_url = make_url(database_config.migration_url)
    alembic_config = Config(str(PROJECT_ROOT / "alembic.ini"))
    migration_scripts = ScriptDirectory.from_config(alembic_config)
    expected_revision = migration_scripts.get_current_head()
    if not expected_revision:
        raise RuntimeError("Tidak ada Alembic head untuk diekspor")

    engine = create_engine(
        migration_url,
        connect_args={"connect_timeout": 10},
        pool_pre_ping=True,
    )
    try:
        with engine.connect() as connection:
            marker = connection.execute(
                text("SELECT shobj_description(oid, 'pg_database') FROM pg_database WHERE datname = current_database()")
            ).scalar_one_or_none()
            if marker != f"algentra:development:{database_config.project_ref}":
                raise RuntimeError("Ekspor ditolak: marker Supabase tidak cocok dengan project development")
            applied_revision = connection.execute(
                text("SELECT version_num FROM public.alembic_version")
            ).scalar_one_or_none()
            if applied_revision != expected_revision:
                raise RuntimeError(
                    "Ekspor ditolak: schema development belum berada pada Alembic head "
                    f"(expected={expected_revision}, applied={applied_revision!r})"
                )
            schema_marker = connection.execute(
                text("SELECT obj_description('app'::regnamespace, 'pg_namespace')")
            ).scalar_one_or_none()
            if schema_marker != f"algentra:revision:{expected_revision}":
                raise RuntimeError("Ekspor ditolak: comment revision schema app tidak cocok dengan Alembic head")
    finally:
        engine.dispose()

    pg_dump = shutil.which("pg_dump")
    if not pg_dump:
        raise RuntimeError("pg_dump tidak ditemukan; pasang PostgreSQL client tools terlebih dahulu")
    destination = destination.expanduser().resolve()
    destination.parent.mkdir(parents=True, exist_ok=True)

    dump_environment = os.environ.copy()
    if migration_url.password:
        dump_environment["PGPASSWORD"] = migration_url.password
    dump_environment["PGSSLMODE"] = migration_url.query.get("sslmode", "require")
    dump_environment["PGCONNECT_TIMEOUT"] = "10"
    command = [
        pg_dump,
        "--host",
        migration_url.host or "",
        "--port",
        str(migration_url.port or 5432),
        "--username",
        migration_url.username or "",
        "--dbname",
        migration_url.database or "postgres",
        "--format=plain",
        "--schema-only",
        "--schema=app",
        "--no-owner",
        "--no-acl",
        "--file",
        str(destination),
    ]
    subprocess.run(command, check=True, env=dump_environment)

    sql_dump = destination.read_text(encoding="utf-8")
    sql_dump += f"""

-- Stamp only the verified Alembic head. This file contains schema DDL, not dev table data.
COMMENT ON SCHEMA app IS 'algentra:revision:{expected_revision}';

GRANT USAGE ON SCHEMA app TO algentra_api, algentra_backup;
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA app TO algentra_api;
GRANT USAGE, SELECT, UPDATE ON ALL SEQUENCES IN SCHEMA app TO algentra_api;
GRANT SELECT ON ALL TABLES IN SCHEMA app TO algentra_backup;
GRANT SELECT ON ALL SEQUENCES IN SCHEMA app TO algentra_backup;
ALTER DEFAULT PRIVILEGES FOR ROLE postgres IN SCHEMA app
    GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO algentra_api;
ALTER DEFAULT PRIVILEGES FOR ROLE postgres IN SCHEMA app
    GRANT USAGE, SELECT, UPDATE ON SEQUENCES TO algentra_api;
ALTER DEFAULT PRIVILEGES FOR ROLE postgres IN SCHEMA app
    GRANT SELECT ON TABLES TO algentra_backup;
ALTER DEFAULT PRIVILEGES FOR ROLE postgres IN SCHEMA app
    GRANT SELECT ON SEQUENCES TO algentra_backup;
"""
    destination.write_text(sql_dump, encoding="utf-8", newline="\n")
    print(f"Snapshot schema {expected_revision} written to {destination}")


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("destination", type=Path, help="Path for the generated production SQL snapshot")
    args = parser.parse_args()
    export_schema_snapshot(args.destination)


if __name__ == "__main__":
    main()
