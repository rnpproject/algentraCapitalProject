import hashlib
import logging
import re
import secrets
from datetime import timedelta
from urllib.parse import urlsplit

from fastapi import APIRouter, Depends, HTTPException, Request, Response
from itsdangerous import BadSignature, SignatureExpired, URLSafeTimedSerializer
from sqlalchemy.orm import Session

from ...core.config import get_settings
from ...services.cs_service import customer_service
from ...database import get_db
from ...models import MT5Account, utc_now
from ...schemas import CSChatMessageRequest
from ...repositories.customer_service import CustomerServiceRepository
from ...core.security import csrf_token, require_csrf


router = APIRouter(prefix="/cs", tags=["customer-service"])
logger = logging.getLogger(__name__)
CONVERSATION_LIFETIME = timedelta(hours=24)
PUBLIC_COOKIE = "alg_cs_visitor"
PUBLIC_COOKIE_MAX_AGE = int(CONVERSATION_LIFETIME.total_seconds())
MAX_RETURNED_MESSAGES = 40
TOKEN_MARKER = re.compile(r"\[\[ALGENTRA_TOKEN:\d+\]\]")
SECRET_LIKE = re.compile(r"(?<![A-Za-z0-9_-])[A-Za-z0-9_-]{32,}(?![A-Za-z0-9_-])")


def _visitor_serializer() -> URLSafeTimedSerializer:
    secret = get_settings().app_secret_key.get_secret_value()
    return URLSafeTimedSerializer(secret, salt="algentra-public-cs-chat-v1")


def _set_visitor_cookie(response: Response, visitor_id: str) -> None:
    settings = get_settings()
    response.set_cookie(
        PUBLIC_COOKIE,
        _visitor_serializer().dumps(visitor_id),
        max_age=PUBLIC_COOKIE_MAX_AGE,
        path="/",
        secure=settings.cookie_secure,
        httponly=True,
        samesite="lax",
    )


def _resolve_identity(
    request: Request, response: Response, db: Session, *, writing: bool
) -> tuple[str, int | None, str | None]:
    client_id = request.session.get("client_user_id")
    if isinstance(client_id, int):
        if not CustomerServiceRepository(db).client_exists(client_id):
            raise HTTPException(status_code=401, detail="Login klien diperlukan")
        if writing:
            require_csrf(request)
        else:
            csrf_token(request)
        return "client", client_id, None
    if request.session.get("admin_username"):
        raise HTTPException(status_code=403, detail="Chat ALGENTRA tersedia untuk pengunjung dan klien")

    visitor_id = None
    raw_cookie = request.cookies.get(PUBLIC_COOKIE)
    if raw_cookie:
        try:
            visitor_id = _visitor_serializer().loads(raw_cookie, max_age=PUBLIC_COOKIE_MAX_AGE)
        except (BadSignature, SignatureExpired):
            visitor_id = None
    if not isinstance(visitor_id, str) or len(visitor_id) < 32:
        visitor_id = secrets.token_urlsafe(32)
    _set_visitor_cookie(response, visitor_id)
    visitor_hash = hashlib.sha256(visitor_id.encode("ascii")).hexdigest()
    return "public", None, visitor_hash


def _check_same_origin(request: Request) -> None:
    source = request.headers.get("origin") or request.headers.get("referer")
    if not source:
        return
    try:
        source_host = urlsplit(source).netloc.casefold()
    except ValueError as exc:
        raise HTTPException(status_code=403, detail="Origin permintaan tidak valid") from exc
    request_host = request.headers.get("host", "").casefold()
    if not source_host or source_host != request_host:
        raise HTTPException(status_code=403, detail="Permintaan chat harus berasal dari situs ini")


def _owned_accounts(db: Session, client_id: int) -> list[MT5Account]:
    return CustomerServiceRepository(db).owned_accounts(client_id)


def _account_context(accounts: list[MT5Account]) -> list[dict[str, object]]:
    # No login numbers, balances, server credentials, or token state go to Gemini.
    return [
        {
            "label": account.label,
            "active": account.active,
            "plan": account.plan,
            "execution_mode": account.execution_mode,
        }
        for account in accounts
    ]


def _sanitize_message(message: str) -> str:
    message = message.strip()
    message = SECRET_LIKE.sub("[nilai rahasia disamarkan]", message)
    return TOKEN_MARKER.sub("[token akun disamarkan]", message)[:2000]


@router.get("/chat/history")
def get_chat_history(request: Request, response: Response, db: Session = Depends(get_db)) -> dict[str, object]:
    mode, client_id, visitor_hash = _resolve_identity(request, response, db, writing=False)
    now = utc_now()
    repository = CustomerServiceRepository(db)
    repository.expire_conversations(now)
    conversation = repository.active_conversation(client_id=client_id, visitor_hash=visitor_hash, now=now)
    messages: list[dict[str, str]] = []
    expires_at = None
    if conversation is not None:
        rows = repository.messages(conversation.id, MAX_RETURNED_MESSAGES)
        rows.reverse()
        messages = [
            {
                "role": row.role,
                "content": TOKEN_MARKER.sub("[token disamarkan; gunakan Rotasi token di Portal Klien]", row.content),
            }
            for row in rows
        ]
        expires_at = conversation.expires_at.isoformat()
    repository.commit()
    return {
        "mode": mode,
        "assistant": "ALGENTRA",
        "messages": messages,
        "expires_at": expires_at,
        "csrf_token": csrf_token(request) if mode == "client" else None,
    }


@router.post("/chat/message")
async def send_chat_message(
    payload: CSChatMessageRequest,
    request: Request,
    response: Response,
    db: Session = Depends(get_db),
) -> dict[str, object]:
    _check_same_origin(request)
    mode, client_id, visitor_hash = _resolve_identity(request, response, db, writing=True)
    message = _sanitize_message(payload.message)
    if not message:
        raise HTTPException(status_code=422, detail="Tulis pesan terlebih dahulu")

    repository = CustomerServiceRepository(db)
    conversation = repository.get_or_create_conversation(
        client_id=client_id,
        visitor_hash=visitor_hash,
        lifetime=CONVERSATION_LIFETIME,
    )
    prior_rows = repository.messages(conversation.id, MAX_RETURNED_MESSAGES)
    prior_rows.reverse()
    history = [{"role": row.role, "content": row.content} for row in prior_rows]
    accounts = _owned_accounts(db, client_id) if client_id is not None else []

    try:
        reply = await customer_service.reply(
            mode=mode,
            message=message,
            history=history,
            account_context=_account_context(accounts) if mode == "client" else None,
        )
    except Exception as exc:
        # Gemini API errors expose a numeric HTTP code (for example 503). Log
        # only that safe classification; exception messages may contain user
        # prompts or other request details.
        upstream_code = getattr(exc, "code", None)
        if isinstance(exc, TimeoutError):
            logger.warning(
                "ALGENTRA CS request failed (TimeoutError, local deadline %.1fs)",
                customer_service.settings.gemini_cs_timeout_seconds,
            )
        elif isinstance(upstream_code, int) and not isinstance(upstream_code, bool):
            logger.warning(
                "ALGENTRA CS request failed (%s, upstream HTTP %d)",
                type(exc).__name__,
                upstream_code,
            )
        else:
            logger.warning("ALGENTRA CS request failed (%s)", type(exc).__name__)
        raise HTTPException(
            status_code=503,
            detail="Chat ALGENTRA sedang tidak tersedia. Silakan coba lagi sebentar.",
        ) from exc

    reply = TOKEN_MARKER.sub("[token disamarkan; gunakan Rotasi token di Portal Klien]", reply).strip()
    reply = SECRET_LIKE.sub("[nilai rahasia disamarkan]", reply)[:4000]

    repository.save_exchange(conversation.id, message, reply)
    return {
        "mode": mode,
        "assistant": "ALGENTRA",
        "message": reply,
        "expires_at": conversation.expires_at.isoformat(),
    }
