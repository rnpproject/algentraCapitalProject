import hashlib

import bcrypt
from sqlalchemy.orm import Session

from .core.config import Settings
from .models import AdminUser


def seed_admin(db: Session, settings: Settings) -> None:
    user = db.get(AdminUser, settings.admin_username)
    password = settings.admin_password.get_secret_value().encode("utf-8")
    if user is None:
        password_hash = bcrypt.hashpw(password, bcrypt.gensalt(rounds=12)).decode("ascii")
        db.add(AdminUser(username=settings.admin_username, password_hash=password_hash))
        db.commit()
    elif not verify_password(password.decode("utf-8"), user.password_hash):
        user.password_hash = bcrypt.hashpw(password, bcrypt.gensalt(rounds=12)).decode("ascii")
        db.commit()


def verify_password(password: str, password_hash: str) -> bool:
    encoded = password.encode("utf-8")
    try:
        stored = password_hash.encode("ascii")
    except (ValueError, UnicodeEncodeError):
        return False
    try:
        if bcrypt.checkpw(encoded, stored):
            return True
    except ValueError:
        # Existing bcrypt hashes reject inputs over 72 bytes; still try the new format.
        pass
    try:
        return bcrypt.checkpw(hashlib.sha256(encoded).digest(), stored)
    except ValueError:
        return False


def hash_password(password: str) -> str:
    # Pre-hash with SHA-256 so bcrypt's 72-byte input limit does not truncate passwords.
    prehashed = hashlib.sha256(password.encode("utf-8")).digest()
    return bcrypt.hashpw(prehashed, bcrypt.gensalt(rounds=12)).decode("ascii")
