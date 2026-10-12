"""Load only the database settings needed by development schema/data tools."""

from dataclasses import dataclass
import os
from urllib.parse import parse_qs, urlsplit

from .core.config import PROJECT_ROOT


@dataclass(frozen=True)
class DevelopmentDatabaseConfig:
    project_ref: str
    runtime_url: str
    migration_url: str


def _development_env_values() -> dict[str, str]:
    env_path = PROJECT_ROOT / ".env.local"
    if not env_path.is_file():
        raise RuntimeError("File .env.local development tidak ditemukan")
    supported_keys = {"APP_ENV", "DATABASE_URL", "MIGRATION_DATABASE_URL", "SUPABASE_DEV_PROJECT_REF"}
    values: dict[str, str] = {}
    for raw_line in env_path.read_text(encoding="utf-8-sig").splitlines():
        line = raw_line.strip()
        if not line or line.startswith("#") or "=" not in line:
            continue
        name, value = line.split("=", maxsplit=1)
        name = name.strip()
        if name not in supported_keys:
            continue
        value = value.strip()
        if len(value) >= 2 and value[0] == value[-1] and value[0] in {"'", '"'}:
            value = value[1:-1]
        values[name] = value

    for name in supported_keys - {"APP_ENV"}:
        process_value = os.environ.get(name, "").strip()
        if process_value:
            values[name] = process_value
    return values


def _normalize_postgres_url(value: str, name: str):
    parsed = urlsplit(value)
    if parsed.scheme.casefold() not in {"postgres", "postgresql", "postgresql+psycopg"}:
        raise RuntimeError(f"{name} harus memakai Supabase PostgreSQL")
    if not parsed.hostname or not parsed.username or not parsed.password or not parsed.path.strip("/"):
        raise RuntimeError(f"{name} harus berupa connection string PostgreSQL lengkap")
    sslmode = parse_qs(parsed.query).get("sslmode", [""])[-1].casefold()
    if sslmode not in {"require", "verify-ca", "verify-full"}:
        raise RuntimeError(f"{name} harus mewajibkan TLS lewat sslmode")
    return parsed


def load_development_database_config() -> DevelopmentDatabaseConfig:
    if os.environ.get("APP_ENV", "development").casefold() != "development":
        raise RuntimeError("Operasi database ini hanya boleh dijalankan dengan APP_ENV=development")
    values = _development_env_values()
    if values.get("APP_ENV", "").casefold() != "development":
        raise RuntimeError(".env.local harus secara eksplisit menetapkan APP_ENV=development untuk tooling migrasi")

    project_ref = values.get("SUPABASE_DEV_PROJECT_REF", "").strip()
    if not project_ref:
        raise RuntimeError("Isi SUPABASE_DEV_PROJECT_REF pada .env.local")
    runtime_value = values.get("DATABASE_URL", "").strip()
    migration_value = values.get("MIGRATION_DATABASE_URL", "").strip()
    if not runtime_value:
        raise RuntimeError("Isi DATABASE_URL role algentra_api pada .env.local")
    if not migration_value:
        raise RuntimeError("Isi MIGRATION_DATABASE_URL role algentra_migrator pada .env.local")

    runtime = _normalize_postgres_url(runtime_value, "DATABASE_URL")
    migrator = _normalize_postgres_url(migration_value, "MIGRATION_DATABASE_URL")
    runtime_role = (runtime.username or "").split(".", maxsplit=1)[0].casefold()
    migration_role = (migrator.username or "").split(".", maxsplit=1)[0].casefold()
    if runtime_role != "algentra_api":
        raise RuntimeError("DATABASE_URL harus memakai role algentra_api")
    if migration_role != "algentra_migrator":
        raise RuntimeError("MIGRATION_DATABASE_URL harus memakai role algentra_migrator")

    project_ref_folded = project_ref.casefold()
    runtime_identity = f"{runtime.hostname or ''} {runtime.username or ''}".casefold()
    migration_identity = f"{migrator.hostname or ''} {migrator.username or ''}".casefold()
    if (
        runtime.path != migrator.path
        or runtime.username == migrator.username
        or project_ref_folded not in runtime_identity
        or project_ref_folded not in migration_identity
    ):
        raise RuntimeError("DATABASE_URL dan MIGRATION_DATABASE_URL harus menunjuk project development yang sama")

    def normalize(value: str) -> str:
        if value.startswith("postgres://"):
            return "postgresql+psycopg://" + value.removeprefix("postgres://")
        if value.startswith("postgresql://"):
            return "postgresql+psycopg://" + value.removeprefix("postgresql://")
        return value

    return DevelopmentDatabaseConfig(
        project_ref=project_ref,
        runtime_url=normalize(runtime_value),
        migration_url=normalize(migration_value),
    )
