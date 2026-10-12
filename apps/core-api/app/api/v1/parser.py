from fastapi import APIRouter, Depends, HTTPException
from pydantic import BaseModel, Field
from sqlalchemy.orm import Session

from ...database import get_db
from ...services.parser_service import preview_signal_classification


router = APIRouter(prefix="/parser", tags=["parser"])


class ParserTestRequest(BaseModel):
    message: str = Field(min_length=1, max_length=8000)
    context: str | None = Field(default=None, max_length=16000)


@router.post("/test")
async def test_parser(payload: ParserTestRequest, db: Session = Depends(get_db)) -> dict[str, object]:
    try:
        return await preview_signal_classification(db, message=payload.message, context=payload.context)
    except RuntimeError as exc:
        raise HTTPException(status_code=503, detail=str(exc)) from exc
