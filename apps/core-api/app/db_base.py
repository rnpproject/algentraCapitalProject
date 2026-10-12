from sqlalchemy.orm import DeclarativeBase


class Base(DeclarativeBase):
    pass


EXPECTED_SCHEMA_REVISION = "0002_rls_and_token_redaction"
