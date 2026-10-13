from pathlib import Path
from urllib.parse import urlsplit, urlunsplit

from fastapi import APIRouter, Depends, HTTPException, Query, Request
from fastapi.responses import HTMLResponse, RedirectResponse
from fastapi.templating import Jinja2Templates
from sqlalchemy.orm import Session

from ...core.config import get_settings
from ...database import get_db
from ...services.public_service import _public_contact_context, public_performance_payload
from ...services.registration import registration_is_open


router = APIRouter(tags=["public"])


def template_context(_: Request) -> dict[str, str | None]:
    return {"public_music_url": get_settings().public_music_url}


templates = Jinja2Templates(
    directory=str(Path(__file__).resolve().parents[2] / "templates"),
    context_processors=[template_context],
)


@router.get("/api/public/performance")
@router.get("/v1/public/performance")
def public_performance(
    db: Session = Depends(get_db),
    include_equity_curve: bool = Query(default=True),
) -> dict[str, object]:
    return public_performance_payload(db, include_equity_curve=include_equity_curve)


@router.get("/api/public/registration-status")
@router.get("/v1/public/registration-status")
def public_registration_status(db: Session = Depends(get_db)) -> dict[str, bool]:
    return {"registration_open": registration_is_open(db)}


@router.get("/musik.mp3", include_in_schema=False)
def site_music() -> RedirectResponse:
    music_url = get_settings().public_music_url
    if not music_url:
        raise HTTPException(status_code=404, detail="Audio publik belum dikonfigurasi")
    parsed_url = urlsplit(music_url)
    path_segments = parsed_url.path.strip("/").split("/")
    if parsed_url.hostname == "github.com" and len(path_segments) >= 5 and path_segments[2] == "blob":
        raw_path = "/" + "/".join((path_segments[0], path_segments[1], *path_segments[3:]))
        music_url = urlunsplit(("https", "raw.githubusercontent.com", raw_path, parsed_url.query, ""))
    return RedirectResponse(music_url, status_code=307)


@router.get("/", response_class=HTMLResponse)
def public_home(request: Request, db: Session = Depends(get_db)):
    return templates.TemplateResponse(request=request, name="public.html", context=_public_contact_context(db))


@router.get("/performance", response_class=HTMLResponse)
def public_performance_page(request: Request, db: Session = Depends(get_db)):
    return templates.TemplateResponse(request=request, name="public.html", context=_public_contact_context(db))
