"""Application settings use cases and validation rules."""

import json
import math
import re

from dotenv import set_key
from sqlalchemy.orm import Session

from ..core.config import PROJECT_ROOT, get_settings
from ..repositories.settings import SettingsRepository
from ..services.gemini_parser import GeminiParser
from .registration import REGISTRATION_AUTO_OPEN_KEY, REGISTRATION_OPEN_KEY, registration_is_open, set_registration_open


ALLOWED_SETTINGS = {
    "confidence_threshold",
    "symbol_mapping",
    "allow_updates",
    "kill_switch",
    "max_market_deviation_pct",
    "registration_open",
    "entry_mode",
}
_SYMBOL_PATTERN = re.compile(r"^[A-Za-z0-9._]{1,32}$")


class SettingsValidationError(ValueError):
    pass


class SettingsConflictError(RuntimeError):
    pass


def _validate_setting(key: str, value: object) -> object:
    if key == "entry_mode":
        if not isinstance(value, str) or value not in {"SINGLE", "PARTIAL"}:
            raise SettingsValidationError("entry_mode harus SINGLE atau PARTIAL")
        return value
    if key in {"allow_updates", "kill_switch", "registration_open"}:
        if not isinstance(value, bool):
            raise SettingsValidationError(f"Setting '{key}' harus boolean")
        return value
    if key in {"confidence_threshold", "max_market_deviation_pct"}:
        if isinstance(value, bool) or not isinstance(value, (int, float)) or not math.isfinite(value):
            raise SettingsValidationError(f"Setting '{key}' harus angka valid")
        ranges = {
            "confidence_threshold": (0.0, 1.0),
            "max_market_deviation_pct": (0.1, 100.0),
        }
        lower, upper = ranges[key]
        if not lower <= value <= upper:
            raise SettingsValidationError(f"Setting '{key}' di luar rentang {lower} sampai {upper}")
        return float(value)
    if key == "symbol_mapping":
        if not isinstance(value, dict) or len(value) > 100:
            raise SettingsValidationError("symbol_mapping harus objek maksimal 100 pasangan")
        normalized = {}
        for source, target in value.items():
            if (
                not isinstance(source, str)
                or not isinstance(target, str)
                or not _SYMBOL_PATTERN.fullmatch(source)
                or not _SYMBOL_PATTERN.fullmatch(target)
            ):
                raise SettingsValidationError("Semua symbol mapping harus berupa symbol valid")
            normalized[source.upper()] = target.upper()
        return normalized
    raise SettingsValidationError(f"Setting '{key}' tidak dikenal")


def read_application_settings(db: Session) -> dict[str, object]:
    values = {}
    for item in SettingsRepository(db).list_all():
        if item.key == REGISTRATION_AUTO_OPEN_KEY or item.key in {
            "default_symbol",
            "max_daily_loss_money",
            "max_lot",
            "max_open_trades",
            "max_signal_age_seconds",
            "allowed_symbols",
            "demo_mode",
        }:
            continue
        try:
            values[item.key] = json.loads(item.value)
        except (TypeError, json.JSONDecodeError):
            continue
    config = get_settings()
    values.setdefault("entry_mode", "PARTIAL")
    values[REGISTRATION_OPEN_KEY] = registration_is_open(db)
    values["gemini_configured"] = bool(config.gemini_api_key and config.gemini_api_key.get_secret_value())
    values["gemini_model"] = config.gemini_model
    return values


def update_application_settings(db: Session, values: dict[str, object]) -> dict[str, object]:
    if len(values) > 100:
        raise SettingsValidationError("Maksimal 100 setting per permintaan")
    app_settings = SettingsRepository(db)
    validated: dict[str, object] = {}
    for key, value in values.items():
        if not key.strip() or len(key) > 128:
            raise SettingsValidationError("Nama setting harus berisi 1-128 karakter")
        if key not in ALLOWED_SETTINGS:
            raise SettingsValidationError(f"Setting '{key}' tidak dapat diubah dari dashboard")
        normalized = _validate_setting(key, value)
        encoded = json.dumps(normalized, ensure_ascii=False, allow_nan=False)
        if len(encoded) > 10_000:
            raise SettingsValidationError(f"Nilai setting '{key}' terlalu besar")
        if key == REGISTRATION_OPEN_KEY:
            set_registration_open(db, normalized)
        else:
            app_settings.set(key, encoded)
        validated[key] = normalized
    db.commit()
    return validated


def update_gemini_config(*, api_key: str | None, model: str | None) -> dict[str, object]:
    if api_key is not None:
        raise SettingsConflictError("Atur GEMINI_API_KEY di environment layanan, lalu mulai ulang aplikasi.")
    if model is not None:
        config = get_settings()
        if config.app_env != "development":
            raise SettingsConflictError(
                "Ubah GEMINI_MODEL pada environment Windows Service, lalu mulai ulang aplikasi."
            )
        env_path = PROJECT_ROOT / ".env.local"
        if not env_path.exists():
            raise SettingsConflictError("Model Gemini hanya dapat diubah bila file .env.local tersedia")
        set_key(str(env_path), "GEMINI_MODEL", model, quote_mode="always")
        get_settings.cache_clear()
    GeminiParser.reload_instances()
    config = get_settings()
    return {
        "gemini_configured": bool(config.gemini_api_key and config.gemini_api_key.get_secret_value()),
        "gemini_model": config.gemini_model,
    }
