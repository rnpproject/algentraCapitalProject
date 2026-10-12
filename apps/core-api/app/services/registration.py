"""Registration availability shared by public pages and auth endpoints."""

import json
from datetime import datetime, timedelta, timezone

from sqlalchemy.orm import Session

from ..repositories.settings import SettingsRepository


REGISTRATION_OPEN_KEY = "registration_open"
REGISTRATION_AUTO_OPEN_KEY = "registration_auto_open"
LAUNCH_AT = datetime(2027, 1, 16, 9, 0, tzinfo=timezone(timedelta(hours=7)))


def registration_is_open(db: Session) -> bool:
    settings = SettingsRepository(db)
    setting = settings.get(REGISTRATION_OPEN_KEY)
    if setting is None:
        return True
    try:
        is_open = json.loads(setting.value) is True
    except (TypeError, json.JSONDecodeError):
        is_open = False
    if is_open:
        return True

    auto_open = settings.get(REGISTRATION_AUTO_OPEN_KEY)
    try:
        should_auto_open = auto_open is not None and json.loads(auto_open.value) is True
    except (TypeError, json.JSONDecodeError):
        should_auto_open = False
    return should_auto_open and datetime.now(timezone.utc) >= LAUNCH_AT.astimezone(timezone.utc)


def registration_launch_pending(db: Session) -> bool:
    if registration_is_open(db):
        return False
    auto_open = SettingsRepository(db).get(REGISTRATION_AUTO_OPEN_KEY)
    try:
        should_auto_open = auto_open is not None and json.loads(auto_open.value) is True
    except (TypeError, json.JSONDecodeError):
        should_auto_open = False
    return should_auto_open and datetime.now(timezone.utc) < LAUNCH_AT.astimezone(timezone.utc)


def set_registration_open(db: Session, is_open: bool) -> None:
    settings = SettingsRepository(db)
    settings.set(REGISTRATION_OPEN_KEY, json.dumps(is_open))

    should_auto_open = not is_open and datetime.now(timezone.utc) < LAUNCH_AT.astimezone(timezone.utc)
    settings.set(REGISTRATION_AUTO_OPEN_KEY, json.dumps(should_auto_open))
