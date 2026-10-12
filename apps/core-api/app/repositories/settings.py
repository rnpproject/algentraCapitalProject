"""SQLAlchemy repository for persistent application settings."""

import json

from sqlalchemy import select
from sqlalchemy.orm import Session

from ..models import AppSetting


class SettingsRepository:
    def __init__(self, session: Session) -> None:
        self._session = session

    def get(self, key: str) -> AppSetting | None:
        return self._session.get(AppSetting, key)

    def get_json(self, key: str, default: object = None) -> object:
        item = self.get(key)
        if item is None:
            return default
        try:
            return json.loads(item.value)
        except (TypeError, json.JSONDecodeError):
            return default

    def list_all(self) -> list[AppSetting]:
        return list(self._session.scalars(select(AppSetting).order_by(AppSetting.key)))

    def set(self, key: str, value: str) -> AppSetting:
        item = self.get(key)
        if item is None:
            item = AppSetting(key=key, value=value)
            self._session.add(item)
        else:
            item.value = value
        return item

    def set_if_missing(self, key: str, value: str) -> AppSetting:
        item = self.get(key)
        return item if item is not None else self.set(key, value)

    def delete(self, key: str) -> bool:
        item = self.get(key)
        if item is None:
            return False
        self._session.delete(item)
        return True
