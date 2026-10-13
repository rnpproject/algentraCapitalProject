from functools import lru_cache
import os
from pathlib import Path
from urllib.parse import parse_qs, urlsplit

from pydantic import Field, SecretStr, model_validator
from pydantic_settings import BaseSettings, SettingsConfigDict


PROJECT_ROOT = Path(__file__).resolve().parents[4]


class Settings(BaseSettings):
    model_config = SettingsConfigDict(
        env_file=None,
        env_ignore_empty=True,
        extra="ignore",
        hide_input_in_errors=True,
    )

    app_secret_key: SecretStr = Field(validation_alias="APP_SECRET_KEY")
    admin_username: str = Field(default="admin", validation_alias="ADMIN_USERNAME")
    admin_password: SecretStr = Field(validation_alias="ADMIN_PASSWORD")
    app_env: str = Field(default="development", validation_alias="APP_ENV")
    cookie_secure: bool = Field(default=True, validation_alias="COOKIE_SECURE")
    gemini_api_key: SecretStr | None = Field(default=None, validation_alias="GEMINI_API_KEY")
    gemini_model: str = Field(default="gemini-3.8-flash", validation_alias="GEMINI_MODEL")
    gemini_cs_api_key: SecretStr | None = Field(default=None, validation_alias="GEMINI_CS_API_KEY")
    gemini_cs_model: str = Field(default="gemini-3.8-flash", validation_alias="GEMINI_CS_MODEL")
    gemini_timeout_seconds: float = Field(default=25.0, validation_alias="GEMINI_TIMEOUT_SECONDS", gt=0, le=120)
    gemini_cs_timeout_seconds: float = Field(default=60.0, validation_alias="GEMINI_CS_TIMEOUT_SECONDS", ge=15, le=180)
    gemini_retry_attempts: int = Field(default=3, validation_alias="GEMINI_RETRY_ATTEMPTS", ge=1, le=6)
    enable_regex_fallback: bool = Field(default=False, validation_alias="ENABLE_REGEX_FALLBACK")
    telegram_api_id: int | None = Field(default=None, validation_alias="TELEGRAM_API_ID", gt=0)
    telegram_api_hash: SecretStr | None = Field(default=None, validation_alias="TELEGRAM_API_HASH")
    telegram_bot_token: SecretStr | None = Field(default=None, validation_alias="TELEGRAM_BOT_TOKEN")
    telegram_admin_chat_id: str | None = Field(default=None, validation_alias="TELEGRAM_ADMIN_CHAT_ID")
    payment_manual_instructions: str = Field(
        default="", validation_alias="PAYMENT_MANUAL_INSTRUCTIONS", max_length=1200
    )
    ea_api_key: SecretStr | None = Field(default=None, validation_alias="EA_API_KEY")
    copier_worker_api_key: SecretStr | None = Field(default=None, validation_alias="COPIER_WORKER_API_KEY")
    bff_hmac_secret: SecretStr | None = Field(default=None, validation_alias="BFF_HMAC_SECRET")
    email_smtp_host: str | None = Field(default=None, validation_alias="EMAIL_SMTP_HOST")
    email_smtp_port: int = Field(default=587, validation_alias="EMAIL_SMTP_PORT", ge=1, le=65535)
    email_smtp_username: str | None = Field(default=None, validation_alias="EMAIL_SMTP_USERNAME")
    email_smtp_password: SecretStr | None = Field(default=None, validation_alias="EMAIL_SMTP_PASSWORD")
    email_from: str | None = Field(default=None, validation_alias="EMAIL_FROM")
    email_smtp_starttls: bool = Field(default=True, validation_alias="EMAIL_SMTP_STARTTLS")
    contact_person_name: str | None = Field(default=None, validation_alias="CONTACT_PERSON_NAME")
    contact_whatsapp: str | None = Field(default=None, validation_alias="CONTACT_WHATSAPP")
    contact_email: str | None = Field(default=None, validation_alias="CONTACT_EMAIL")
    contact_instagram: str | None = Field(default=None, validation_alias="CONTACT_INSTAGRAM")
    contact_telegram: str | None = Field(default=None, validation_alias="CONTACT_TELEGRAM")
    contact_telegram_admin: str | None = Field(default=None, validation_alias="CONTACT_TELEGRAM_ADMIN")
    contact_website: str | None = Field(default=None, validation_alias="CONTACT_WEBSITE")
    contact_facebook: str | None = Field(default=None, validation_alias="CONTACT_FACEBOOK")
    contact_linkedin: str | None = Field(default=None, validation_alias="CONTACT_LINKEDIN")
    contact_x: str | None = Field(default=None, validation_alias="CONTACT_X")
    contact_youtube: str | None = Field(default=None, validation_alias="CONTACT_YOUTUBE")
    contact_tiktok: str | None = Field(default=None, validation_alias="CONTACT_TIKTOK")
    public_music_url: str | None = Field(default=None, validation_alias="PUBLIC_MUSIC_URL")
    websocket_allowed_origins: str = Field(default="", validation_alias="WEBSOCKET_ALLOWED_ORIGINS")
    database_url: SecretStr | None = Field(default=None, validation_alias="DATABASE_URL")
    migration_database_url: SecretStr | None = Field(default=None, validation_alias="MIGRATION_DATABASE_URL")
    supabase_dev_project_ref: str | None = Field(default=None, validation_alias="SUPABASE_DEV_PROJECT_REF")
    supabase_project_ref: str | None = Field(default=None, validation_alias="SUPABASE_PROJECT_REF")

    @model_validator(mode="after")
    def validate_credentials(self) -> "Settings":
        if self.app_env not in {"production", "development", "test"}:
            raise ValueError("APP_ENV harus production, development, atau test")
        if self.app_env == "production" and os.environ.get("APP_ENV", "").casefold() != "production":
            raise ValueError("APP_ENV=production harus disetel pada environment proses, bukan file lokal")
        if self.app_env == "production" and not self.cookie_secure:
            raise ValueError("COOKIE_SECURE tidak boleh false di production")
        if self.app_env == "production" and not self.database_url:
            raise ValueError("DATABASE_URL wajib diisi di production")
        if self.app_env == "production" and not (self.supabase_project_ref or "").strip():
            raise ValueError("SUPABASE_PROJECT_REF wajib diisi di production untuk memeriksa database target")
        if self.app_env == "production" and self.bff_hmac_secret is None:
            raise ValueError("BFF_HMAC_SECRET wajib diisi di production untuk memverifikasi permintaan Vercel")
        if self.bff_hmac_secret is not None and len(self.bff_hmac_secret.get_secret_value()) < 32:
            raise ValueError("BFF_HMAC_SECRET minimal 32 karakter")
        if self.app_env == "production" and self.migration_database_url:
            raise ValueError("MIGRATION_DATABASE_URL tidak boleh dipasang pada service production")
        app_secret = self.app_secret_key.get_secret_value()
        admin_password = self.admin_password.get_secret_value()
        if len(app_secret) < 32:
            raise ValueError("APP_SECRET_KEY must contain at least 32 characters")
        if any(
            marker in app_secret.casefold() for marker in ("replace-with", "change-this", "changethis", "your-secret")
        ):
            raise ValueError("APP_SECRET_KEY masih memakai placeholder; ganti dengan secret acak")
        if len(admin_password) < 12:
            raise ValueError("ADMIN_PASSWORD must contain at least 12 characters")
        if any(
            marker in admin_password.casefold()
            for marker in ("replace-with", "change-this", "changethis", "your-password")
        ):
            raise ValueError("ADMIN_PASSWORD masih memakai placeholder; ganti sebelum menjalankan aplikasi")
        if not self.admin_username.strip():
            raise ValueError("ADMIN_USERNAME cannot be empty")
        if not self.gemini_model.strip():
            raise ValueError("GEMINI_MODEL cannot be empty")
        if not self.gemini_cs_model.strip():
            raise ValueError("GEMINI_CS_MODEL cannot be empty")
        if self.public_music_url is not None:
            self.public_music_url = self.public_music_url.strip()
            if not self.public_music_url:
                self.public_music_url = None
            else:
                music_url = urlsplit(self.public_music_url)
                try:
                    music_url.port
                except ValueError as exc:
                    raise ValueError("PUBLIC_MUSIC_URL harus berupa URL HTTPS yang valid") from exc
                if music_url.scheme != "https" or not music_url.hostname or music_url.username or music_url.password:
                    raise ValueError("PUBLIC_MUSIC_URL harus berupa URL HTTPS yang valid")
        allowed_origins: list[str] = []
        for value in self.websocket_allowed_origins.split(","):
            origin = value.strip().rstrip("/")
            if not origin:
                continue
            parsed_origin = urlsplit(origin)
            try:
                parsed_origin.port
            except ValueError as exc:
                raise ValueError("WEBSOCKET_ALLOWED_ORIGINS berisi origin yang tidak valid") from exc
            if (
                parsed_origin.scheme not in {"http", "https"}
                or not parsed_origin.hostname
                or parsed_origin.username
                or parsed_origin.password
                or parsed_origin.path
                or parsed_origin.query
                or parsed_origin.fragment
            ):
                raise ValueError("WEBSOCKET_ALLOWED_ORIGINS harus berupa origin HTTP(S) tanpa path")
            allowed_origins.append(f"{parsed_origin.scheme.casefold()}://{parsed_origin.netloc.casefold()}")
        self.websocket_allowed_origins = ",".join(dict.fromkeys(allowed_origins))
        if (self.telegram_api_id is None) != (self.telegram_api_hash is None):
            raise ValueError("TELEGRAM_API_ID dan TELEGRAM_API_HASH harus diisi bersamaan")
        if (self.telegram_bot_token is None) != (self.telegram_admin_chat_id is None):
            raise ValueError("TELEGRAM_BOT_TOKEN dan TELEGRAM_ADMIN_CHAT_ID harus diisi bersamaan")
        self.payment_manual_instructions = self.payment_manual_instructions.strip()
        if self.telegram_admin_chat_id is not None:
            chat_id = self.telegram_admin_chat_id.strip()
            if not chat_id.isdigit() or int(chat_id) < 1:
                raise ValueError("TELEGRAM_ADMIN_CHAT_ID harus berupa ID chat pribadi admin yang positif")
        if self.database_url:
            database_url = self.database_url.get_secret_value().strip()
            database_scheme = urlsplit(database_url).scheme.casefold()
            if database_scheme in {"postgres", "postgresql", "postgresql+psycopg"}:
                if self.app_env == "test":
                    raise ValueError("APP_ENV=test hanya boleh memakai SQLite terisolasi")
                parsed_database_url = urlsplit(database_url)
                if (
                    not parsed_database_url.hostname
                    or not parsed_database_url.username
                    or not parsed_database_url.password
                    or not parsed_database_url.path.strip("/")
                ):
                    raise ValueError("DATABASE_URL harus berupa connection string PostgreSQL yang lengkap")
                runtime_role = parsed_database_url.username.split(".", maxsplit=1)[0].casefold()
                if runtime_role != "algentra_api":
                    raise ValueError("DATABASE_URL harus memakai role runtime algentra_api, bukan role project owner")
                if self.app_env == "development" and not (self.supabase_dev_project_ref or "").strip():
                    raise ValueError("SUPABASE_DEV_PROJECT_REF wajib diisi untuk koneksi Supabase development")
                sslmode = parse_qs(parsed_database_url.query).get("sslmode", [""])[-1].casefold()
                if sslmode not in {"require", "verify-ca", "verify-full"}:
                    raise ValueError("DATABASE_URL Supabase harus mewajibkan TLS lewat sslmode")
                expected_ref = (
                    (self.supabase_dev_project_ref or "").strip()
                    if self.app_env == "development"
                    else (self.supabase_project_ref or "").strip()
                )
                connection_identity = (
                    (parsed_database_url.hostname or "") + " " + (parsed_database_url.username or "")
                ).casefold()
                if expected_ref and expected_ref.casefold() not in connection_identity:
                    raise ValueError("DATABASE_URL harus menunjuk project Supabase yang cocok dengan project ref")
            elif database_scheme == "sqlite":
                if self.app_env == "production":
                    raise ValueError("Production wajib memakai Supabase PostgreSQL")
            else:
                raise ValueError("DATABASE_URL harus memakai PostgreSQL Supabase atau SQLite untuk development/test")
        if self.migration_database_url:
            if self.app_env != "development":
                raise ValueError("MIGRATION_DATABASE_URL hanya boleh dipakai dalam development")
            migration_url = self.migration_database_url.get_secret_value().strip()
            parsed_migration_url = urlsplit(migration_url)
            if parsed_migration_url.scheme.casefold() not in {"postgres", "postgresql", "postgresql+psycopg"}:
                raise ValueError("MIGRATION_DATABASE_URL harus memakai PostgreSQL Supabase development")
            if (
                not parsed_migration_url.hostname
                or not parsed_migration_url.username
                or not parsed_migration_url.password
            ):
                raise ValueError("MIGRATION_DATABASE_URL harus berupa connection string PostgreSQL lengkap")
            migrator_role = parsed_migration_url.username.split(".", maxsplit=1)[0].casefold()
            if migrator_role != "algentra_migrator":
                raise ValueError("MIGRATION_DATABASE_URL harus memakai role development algentra_migrator")
            sslmode = parse_qs(parsed_migration_url.query).get("sslmode", [""])[-1].casefold()
            if sslmode not in {"require", "verify-ca", "verify-full"}:
                raise ValueError("MIGRATION_DATABASE_URL harus mewajibkan TLS lewat sslmode")
            if not self.database_url:
                raise ValueError(
                    "Isi DATABASE_URL role runtime agar app dan migrator menunjuk project development yang sama"
                )
            runtime_url = urlsplit(self.database_url.get_secret_value().strip())
            project_ref = (self.supabase_dev_project_ref or "").strip().casefold()
            if not project_ref:
                raise ValueError("SUPABASE_DEV_PROJECT_REF wajib diisi untuk menjalankan migrasi development")
            runtime_matches_project = project_ref in (runtime_url.hostname or "").casefold() or (
                runtime_url.username or ""
            ).casefold().endswith(f".{project_ref}")
            migration_matches_project = project_ref in (parsed_migration_url.hostname or "").casefold() or (
                parsed_migration_url.username or ""
            ).casefold().endswith(f".{project_ref}")
            if (
                runtime_url.path != parsed_migration_url.path
                or runtime_url.username == parsed_migration_url.username
                or not runtime_matches_project
                or not migration_matches_project
            ):
                raise ValueError(
                    "DATABASE_URL dan MIGRATION_DATABASE_URL harus menunjuk project development yang sama dengan role berbeda"
                )
        if self.ea_api_key is not None:
            ea_key = self.ea_api_key.get_secret_value()
            if len(ea_key) < 24:
                raise ValueError("EA_API_KEY must contain at least 24 characters")
            if any(
                marker in ea_key.casefold() for marker in ("replace-with", "change-this", "changethis", "your-api-key")
            ):
                raise ValueError("EA_API_KEY masih memakai placeholder; ganti sebelum menjalankan EA")
        if self.copier_worker_api_key is not None:
            worker_key = self.copier_worker_api_key.get_secret_value()
            if len(worker_key) < 32:
                raise ValueError("COPIER_WORKER_API_KEY minimal 32 karakter")
            if any(marker in worker_key.casefold() for marker in ("replace-with", "change-this", "your-api-key")):
                raise ValueError("COPIER_WORKER_API_KEY masih memakai placeholder")
        smtp_values = (self.email_smtp_host, self.email_smtp_username, self.email_smtp_password, self.email_from)
        if any(smtp_values) and not all(smtp_values):
            raise ValueError(
                "EMAIL_SMTP_HOST, EMAIL_SMTP_USERNAME, EMAIL_SMTP_PASSWORD, dan EMAIL_FROM harus diisi bersama"
            )
        return self

    @property
    def email_configured(self) -> bool:
        return all((self.email_smtp_host, self.email_smtp_username, self.email_smtp_password, self.email_from))

    @property
    def resolved_database_url(self) -> str:
        if self.database_url:
            url = self.database_url.get_secret_value().strip()
            if url.startswith("postgres://"):
                return "postgresql+psycopg://" + url.removeprefix("postgres://")
            if url.startswith("postgresql://"):
                return "postgresql+psycopg://" + url.removeprefix("postgresql://")
            return url
        return f"sqlite:///{(PROJECT_ROOT / 'backend' / 'data' / 'app.db').as_posix()}"

    @property
    def resolved_migration_database_url(self) -> str | None:
        if self.migration_database_url is None:
            return None
        url = self.migration_database_url.get_secret_value().strip()
        if url.startswith("postgres://"):
            return "postgresql+psycopg://" + url.removeprefix("postgres://")
        if url.startswith("postgresql://"):
            return "postgresql+psycopg://" + url.removeprefix("postgresql://")
        return url


@lru_cache
def get_settings() -> Settings:
    app_env = os.environ.get("APP_ENV", "development").casefold()
    env_file = PROJECT_ROOT / ".env.local" if app_env == "development" else None
    return Settings(_env_file=env_file)
