import re

from fastapi import APIRouter, Depends, HTTPException
from pydantic import BaseModel, Field, model_validator
from sqlalchemy.orm import Session

from ...database import get_db
from ...schemas import SettingsUpdate
from ...services.settings_service import (
    SettingsConflictError,
    SettingsValidationError,
    read_application_settings,
    update_application_settings,
    update_gemini_config,
)


router = APIRouter(prefix="/settings", tags=["settings"])


class GeminiConfigUpdate(BaseModel):
    api_key: str | None = Field(default=None, min_length=20, max_length=1024)
    model: str | None = Field(default=None, min_length=2, max_length=100)

    @model_validator(mode="after")
    def validate_model_name(self):
        if self.model is not None and not re.fullmatch(r"[A-Za-z0-9._-]+", self.model):
            raise ValueError("Nama model hanya boleh berisi huruf, angka, titik, garis bawah, dan tanda hubung")
        if self.api_key is None and self.model is None:
            raise ValueError("Isi api_key atau model")
        return self


@router.get("")
def read_settings(db: Session = Depends(get_db)) -> dict[str, object]:
    return read_application_settings(db)


@router.put("/gemini-config")
def update_gemini_settings(payload: GeminiConfigUpdate) -> dict[str, object]:
    try:
        return update_gemini_config(api_key=payload.api_key, model=payload.model)
    except SettingsConflictError as exc:
        raise HTTPException(status_code=409, detail=str(exc)) from exc


@router.put("")
def update_settings(payload: SettingsUpdate, db: Session = Depends(get_db)) -> dict[str, object]:
    try:
        return update_application_settings(db, payload.values)
    except SettingsValidationError as exc:
        raise HTTPException(status_code=422, detail=str(exc)) from exc
