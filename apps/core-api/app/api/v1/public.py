import asyncio
import json
from urllib.parse import urlsplit, urlunsplit

from fastapi import APIRouter, Depends, HTTPException, Query, Request
from fastapi.responses import RedirectResponse, StreamingResponse
from sqlalchemy.orm import Session

from ...core.config import get_settings
from ...database import SessionLocal, get_db
from ...services.public_service import public_performance_payload
from ...services.registration import registration_is_open


router = APIRouter(tags=["public"])


def _sse_performance(payload: dict[str, object]) -> bytes:
    return f"event: performance\ndata: {json.dumps(payload, separators=(',', ':'), default=str)}\n\n".encode()


@router.get("/api/public/performance/stream")
@router.get("/v1/public/performance/stream")
async def public_performance_stream(request: Request) -> StreamingResponse:
    """Stream current public performance snapshots; EventSource handles reconnects."""

    async def events():
        last_payload: dict[str, object] | None = None
        last_history_at = 0.0
        while not await request.is_disconnected():
            now = asyncio.get_running_loop().time()
            include_history = last_payload is None or now - last_history_at >= 60

            def load_payload():
                with SessionLocal() as db:
                    return public_performance_payload(db, include_equity_curve=include_history)

            payload = await asyncio.to_thread(load_payload)
            if include_history:
                last_history_at = now
            if payload != last_payload:
                yield _sse_performance(payload)
                last_payload = payload
            else:
                yield b": heartbeat\n\n"
            await asyncio.sleep(2)

    return StreamingResponse(
        events(),
        media_type="text/event-stream",
        headers={"Cache-Control": "public, no-store, no-transform", "X-Accel-Buffering": "no"},
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


@router.get("/api/public/contact")
@router.get("/v1/public/contact")
def public_contact() -> dict[str, object]:
    settings = get_settings()
    socials: list[dict[str, str]] = []
    for field, icon, label in (
        ("contact_instagram", "instagram", "Instagram"),
        ("contact_telegram", "telegram", "Telegram"),
        ("contact_tiktok", "tiktok", "TikTok"),
        ("contact_website", "website", "Website"),
        ("contact_facebook", "generic", "Facebook"),
        ("contact_linkedin", "generic", "LinkedIn"),
        ("contact_x", "generic", "X"),
        ("contact_youtube", "generic", "YouTube"),
    ):
        url = (getattr(settings, field, None) or "").strip()
        if not url:
            continue
        if "://" not in url:
            url = "https://" + url
        display = urlsplit(url).path.strip("/").split("/")[-1]
        if display and not display.startswith("@"):
            display = f"@{display}"
        if not display:
            display = urlsplit(url).hostname or url
        socials.append({"url": url, "icon": icon, "label": label, "display": display})
    return {
        "person": settings.contact_person_name,
        "whatsapp": settings.contact_whatsapp,
        "email": settings.contact_email,
        "socials": socials,
    }


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
