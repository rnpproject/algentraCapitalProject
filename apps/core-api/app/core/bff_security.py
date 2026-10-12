"""HMAC verification for requests sent by the Vercel backend-for-frontend."""

import hashlib
import hmac
import json
import threading
import time

from starlette.types import ASGIApp, Message, Receive, Scope, Send


MAX_SIGNED_BODY_BYTES = 6 * 1024 * 1024
MAX_CLOCK_SKEW_SECONDS = 60
NONCE_TTL_SECONDS = 120


def signature_payload(timestamp: str, nonce: str, method: str, target: str, body: bytes) -> bytes:
    body_hash = hashlib.sha256(body).hexdigest()
    return f"{timestamp}\n{nonce}\n{method.upper()}\n{target}\n{body_hash}".encode("utf-8")


class BFFSignatureMiddleware:
    """Require a fresh HMAC signature for Vercel-originated versioned API calls."""

    _used_nonces: dict[str, float] = {}
    _nonce_lock = threading.Lock()

    def __init__(self, app: ASGIApp, secret: str) -> None:
        self.app = app
        self.secret = secret.encode("utf-8")

    @staticmethod
    async def _reject(send: Send, status: int, detail: str) -> None:
        body = json.dumps({"detail": detail}, separators=(",", ":")).encode("utf-8")
        await send(
            {
                "type": "http.response.start",
                "status": status,
                "headers": [
                    (b"content-type", b"application/json"),
                    (b"content-length", str(len(body)).encode("ascii")),
                ],
            }
        )
        await send({"type": "http.response.body", "body": body})

    @classmethod
    def _claim_nonce(cls, nonce: str, now: float) -> bool:
        with cls._nonce_lock:
            cls._used_nonces = {key: expiry for key, expiry in cls._used_nonces.items() if expiry > now}
            if nonce in cls._used_nonces or len(cls._used_nonces) >= 100_000:
                return False
            cls._used_nonces[nonce] = now + NONCE_TTL_SECONDS
            return True

    async def __call__(self, scope: Scope, receive: Receive, send: Send) -> None:
        if scope["type"] != "http":
            await self.app(scope, receive, send)
            return
        path = scope.get("path", "")
        if not path.startswith("/v1/"):
            await self.app(scope, receive, send)
            return

        body_messages: list[Message] = []
        body_parts: list[bytes] = []
        size = 0
        while True:
            message = await receive()
            if message["type"] == "http.disconnect":
                return
            body_messages.append(message)
            body = message.get("body", b"")
            size += len(body)
            if size > MAX_SIGNED_BODY_BYTES:
                await self._reject(send, 413, "Ukuran permintaan melewati batas")
                return
            body_parts.append(body)
            if not message.get("more_body", False):
                break
        body_bytes = b"".join(body_parts)
        headers = {key.decode("latin-1").casefold(): value.decode("latin-1") for key, value in scope.get("headers", [])}
        timestamp = headers.get("x-algentra-timestamp", "")
        nonce = headers.get("x-algentra-nonce", "")
        supplied_signature = headers.get("x-algentra-signature", "")
        try:
            timestamp_value = int(timestamp)
        except ValueError:
            await self._reject(send, 401, "Tanda tangan BFF tidak valid")
            return
        now = time.time()
        if abs(now - timestamp_value) > MAX_CLOCK_SKEW_SECONDS or not 16 <= len(nonce) <= 128:
            await self._reject(send, 401, "Tanda tangan BFF kedaluwarsa atau nonce tidak valid")
            return
        query_string = scope.get("query_string", b"").decode("latin-1")
        target = path + (f"?{query_string}" if query_string else "")
        expected = hmac.new(
            self.secret,
            signature_payload(timestamp, nonce, scope.get("method", "GET"), target, body_bytes),
            hashlib.sha256,
        ).hexdigest()
        if not hmac.compare_digest(supplied_signature, expected):
            await self._reject(send, 401, "Tanda tangan BFF tidak valid")
            return
        if not self._claim_nonce(nonce, now):
            await self._reject(send, 409, "Permintaan BFF terdeteksi berulang")
            return

        message_index = 0

        async def replay_receive() -> Message:
            nonlocal message_index
            if message_index < len(body_messages):
                message = body_messages[message_index]
                message_index += 1
                return message
            return await receive()

        await self.app(scope, replay_receive, send)
