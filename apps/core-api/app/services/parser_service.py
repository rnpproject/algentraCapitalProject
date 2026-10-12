"""Application use case for the administrator's signal parser preview."""

from sqlalchemy.orm import Session

from ..repositories.settings import SettingsRepository
from .gemini_parser import GeminiParser
from .signal_service import validate_classification


_parser = GeminiParser()


async def preview_signal_classification(
    db: Session,
    *,
    message: str,
    context: str | None = None,
) -> dict[str, object]:
    classification = await _parser.parse(message, context)
    settings = SettingsRepository(db)
    mapping = settings.get_json("symbol_mapping", {})
    result = validate_classification(
        classification,
        default_symbol="XAUUSD",
        confidence_threshold=float(settings.get_json("confidence_threshold", 0.75)),
        symbol_mapping=mapping if isinstance(mapping, dict) else {},
        allow_updates=bool(settings.get_json("allow_updates", False)),
    )
    return {
        "classification": classification.model_dump(mode="json"),
        "status": result.status,
        "symbol": result.symbol,
        "normalized_text": result.normalized_text,
        "validation_reason": result.reason,
    }
