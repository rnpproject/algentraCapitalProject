"""Create encrypted Supabase PostgreSQL backups or local SQLite development backups."""

from datetime import datetime, timezone
import os
from pathlib import Path
import re
import sqlite3
import subprocess
import tempfile

from sqlalchemy.engine import make_url

from .core.config import PROJECT_ROOT, get_settings


SQLITE_BACKUP_NAME = re.compile(r"^algentra-app-\d{8}T\d{6}Z\.db$")
POSTGRES_BACKUP_NAME = re.compile(r"^algentra-postgres-\d{8}T\d{6}Z\.dump\.age$")
BACKUP_RETENTION = 14


def create_backup() -> Path:
    settings = get_settings()
    database_url = settings.resolved_database_url
    backup_dir_value = os.environ.get("BACKUP_DIR", "").strip()
    if not backup_dir_value:
        raise RuntimeError("BACKUP_DIR wajib diarahkan ke lokasi backup persisten")
    backup_dir = Path(backup_dir_value).expanduser().resolve()
    if backup_dir.is_relative_to(PROJECT_ROOT.resolve()):
        raise RuntimeError("BACKUP_DIR harus berada di luar folder aplikasi")
    backup_dir.mkdir(parents=True, exist_ok=True)

    timestamp = datetime.now(timezone.utc).strftime("%Y%m%dT%H%M%SZ")
    if database_url.startswith("sqlite:///"):
        destination = _create_sqlite_backup(database_url, backup_dir, timestamp)
        _rotate_backups(backup_dir, SQLITE_BACKUP_NAME)
        return destination
    if database_url.startswith("postgresql+psycopg://"):
        destination = _create_postgres_backup(backup_dir, timestamp)
        _rotate_backups(backup_dir, POSTGRES_BACKUP_NAME)
        return destination
    raise RuntimeError("DATABASE_URL harus memakai PostgreSQL Supabase atau SQLite development")


def _create_sqlite_backup(database_url: str, backup_dir: Path, timestamp: str) -> Path:
    database_path = Path(database_url.removeprefix("sqlite:///"))
    if not database_path.is_absolute():
        raise RuntimeError("DATABASE_URL SQLite harus berupa jalur absolut")
    database_path = database_path.resolve()
    if not database_path.is_file():
        raise RuntimeError("Database SQLite belum ada; backup tidak dibuat")
    if backup_dir == database_path.parent or backup_dir in database_path.parents:
        raise RuntimeError("BACKUP_DIR harus berada di luar folder database")
    destination = backup_dir / f"algentra-app-{timestamp}.db"
    with sqlite3.connect(database_path) as source, sqlite3.connect(destination) as target:
        source.backup(target)
        integrity = target.execute("PRAGMA quick_check").fetchone()
        if integrity is None or integrity[0] != "ok":
            destination.unlink(missing_ok=True)
            raise RuntimeError("Pemeriksaan integritas backup gagal")
    return destination


def _create_postgres_backup(backup_dir: Path, timestamp: str) -> Path:
    settings = get_settings()
    backup_url_value = os.environ.get("BACKUP_DATABASE_URL", "").strip()
    if settings.app_env == "production" and not backup_url_value:
        raise RuntimeError("BACKUP_DATABASE_URL wajib memakai role backup read-only di production")
    backup_url_value = backup_url_value or settings.resolved_database_url
    backup_url = make_url(backup_url_value)
    if backup_url.get_backend_name() != "postgresql" or not backup_url.host or not backup_url.database:
        raise RuntimeError("BACKUP_DATABASE_URL harus berupa connection string PostgreSQL lengkap")
    if not backup_url.username or not backup_url.password:
        raise RuntimeError("BACKUP_DATABASE_URL harus berisi kredensial role backup")
    if backup_url.username == make_url(settings.resolved_database_url).username:
        raise RuntimeError("BACKUP_DATABASE_URL harus memakai role read-only terpisah dari role runtime")
    sslmode_value = backup_url.query.get("sslmode", "")
    if isinstance(sslmode_value, (tuple, list)):
        sslmode_value = sslmode_value[-1]
    if sslmode_value not in {"require", "verify-ca", "verify-full"}:
        raise RuntimeError("BACKUP_DATABASE_URL harus mewajibkan TLS lewat sslmode")

    recipient = os.environ.get("BACKUP_AGE_RECIPIENT", "").strip()
    if not recipient:
        raise RuntimeError("BACKUP_AGE_RECIPIENT wajib diisi agar pg_dump dienkripsi")

    destination = backup_dir / f"algentra-postgres-{timestamp}.dump.age"
    descriptor, temporary_dump_name = tempfile.mkstemp(prefix="algentra-postgres-", suffix=".dump", dir=backup_dir)
    os.close(descriptor)
    temporary_dump = Path(temporary_dump_name)
    libpq_environment = os.environ.copy()
    libpq_environment.update(
        {
            "PGHOST": backup_url.host,
            "PGPORT": str(backup_url.port or 5432),
            "PGUSER": backup_url.username,
            "PGPASSWORD": backup_url.password,
            "PGDATABASE": backup_url.database,
            "PGSSLMODE": sslmode_value,
        }
    )
    command_environment = {
        key: value
        for key, value in libpq_environment.items()
        if key.startswith("PG") or key in {"PATH", "SYSTEMROOT", "WINDIR", "TEMP", "TMP"}
    }
    try:
        subprocess.run(
            ["pg_dump", "--no-owner", "--no-acl", "--format=custom", "--schema=app", "--file", str(temporary_dump)],
            check=True,
            capture_output=True,
            text=True,
            env=command_environment,
        )
        subprocess.run(
            ["age", "--encrypt", "--recipient", recipient, "--output", str(destination), str(temporary_dump)],
            check=True,
            capture_output=True,
            text=True,
            env=command_environment,
        )
        if not destination.is_file() or destination.stat().st_size == 0:
            raise RuntimeError("File backup Supabase terenkripsi kosong")
        return destination
    except FileNotFoundError as exc:
        destination.unlink(missing_ok=True)
        raise RuntimeError(f"Executable backup tidak ditemukan: {exc.filename}") from exc
    except subprocess.CalledProcessError as exc:
        destination.unlink(missing_ok=True)
        detail = (exc.stderr or "").strip()
        raise RuntimeError(f"Backup Supabase gagal ({exc.cmd[0]}): {detail[:500]}") from exc
    finally:
        temporary_dump.unlink(missing_ok=True)


def _rotate_backups(backup_dir: Path, backup_pattern: re.Pattern[str]) -> None:
    backups = sorted(
        (path for path in backup_dir.iterdir() if backup_pattern.fullmatch(path.name)),
        key=lambda path: path.name,
        reverse=True,
    )
    for expired in backups[BACKUP_RETENTION:]:
        expired.unlink()


if __name__ == "__main__":
    print(create_backup())
