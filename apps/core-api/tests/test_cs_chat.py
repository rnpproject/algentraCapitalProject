import asyncio
from datetime import timedelta
from secrets import token_urlsafe
from types import SimpleNamespace
from uuid import uuid4

from fastapi.testclient import TestClient
from sqlalchemy import delete

from app.auth import hash_password
from app.core.config import Settings
from app.services.cs_service import GeminiCustomerService
from app.database import SessionLocal
from app.main import app
from app.models import CSChatMessage, CSConversation, ClientUser, MT5Account, utc_now
from app.api.v1 import cs as cs_router
from app.api.v1.cs import _sanitize_message
from app.api.v1.mt5 import _token_hash


def _settings(**overrides):
    values = {
        "_env_file": None,
        "APP_SECRET_KEY": "cs-test-secret-key-with-at-least-32-characters",
        "ADMIN_USERNAME": "admin",
        "ADMIN_PASSWORD": "CS-testing-password-123",
        "GEMINI_API_KEY": "signal-parser-key-is-not-for-cs",
        "GEMINI_MODEL": "signal-model",
        "GEMINI_CS_API_KEY": "dedicated-customer-service-key",
        "GEMINI_CS_MODEL": "customer-service-model",
        "GEMINI_TIMEOUT_SECONDS": 5,
    }
    values.update(overrides)
    return Settings(**values)


def _seed_ea_account(owner_id: int, label: str, server: str, login: str) -> tuple[int, str]:
    token = token_urlsafe(32)
    with SessionLocal() as db:
        account = MT5Account(
            owner_id=owner_id,
            label=label,
            server=server,
            login=login,
            plan="ZERO",
            role="follower",
            execution_mode="EA",
            token_hash=_token_hash(token),
            active=True,
        )
        db.add(account)
        db.commit()
        db.refresh(account)
        return account.id, token


def test_ea_token_is_represented_by_a_one_way_hash():
    token = "sample-mt5-account-token-for-encryption"
    digest = _token_hash(token)

    assert digest != token
    assert len(digest) == 64


def test_chat_sanitizes_legacy_token_markers():
    sanitized = _sanitize_message("Token saya [[ALGENTRA_TOKEN:12]]")

    assert "[[ALGENTRA_TOKEN:12]]" not in sanitized
    assert "token akun disamarkan" in sanitized


def test_gemini_customer_service_uses_its_separate_key_and_model():
    calls = []

    class FakeModels:
        def generate_content(self, **kwargs):
            calls.append(kwargs)
            return SimpleNamespace(text="Silakan buka portal klien.")

    client = SimpleNamespace(models=FakeModels())
    keys_used = []
    service = GeminiCustomerService(
        _settings(),
        client_factory=lambda api_key: (keys_used.append(api_key) or client),
    )
    answer = asyncio.run(service.reply(
        mode="public",
        message="Bagaimana cara daftar?",
        history=[],
    ))

    assert answer == "Silakan buka portal klien."
    assert keys_used == ["dedicated-customer-service-key"]
    assert keys_used[0] != _settings().gemini_api_key.get_secret_value()
    assert calls[0]["model"] == "customer-service-model"
    assert "Bagaimana cara daftar?" in calls[0]["contents"]
    thinking_level = calls[0]["config"].thinking_config.thinking_level
    assert getattr(thinking_level, "value", thinking_level) == "LOW"


def test_customer_service_timeout_is_independent_from_signal_parser_timeout():
    configured = _settings(GEMINI_TIMEOUT_SECONDS=25)

    assert configured.gemini_timeout_seconds == 25
    assert configured.gemini_cs_timeout_seconds == 60


def test_chat_failure_logs_upstream_status_without_exception_details(monkeypatch, caplog):
    async def failing_reply(**_kwargs):
        error = RuntimeError("private prompt text must not enter logs")
        error.code = 503
        raise error

    monkeypatch.setattr(cs_router.customer_service, "reply", failing_reply)
    with TestClient(app) as client, caplog.at_level("WARNING", logger=cs_router.logger.name):
        response = client.post("/api/cs/chat/message", json={"message": "Hai"})

    assert response.status_code == 503
    assert "upstream HTTP 503" in caplog.text
    assert "private prompt text must not enter logs" not in caplog.text


def test_client_chat_is_private_and_never_returns_their_token(monkeypatch):
    email = f"cs-client-{uuid4().hex}@example.com"
    observed = []

    async def fake_reply(**kwargs):
        observed.append(kwargs)
        return "Buka pengaturan EA Copy Trading di MT5."

    monkeypatch.setattr(cs_router.customer_service, "reply", fake_reply)
    with TestClient(app) as client:
        # Create a public conversation first; logging in must switch to a different history.
        public_history = client.get("/api/cs/chat/history")
        assert public_history.status_code == 200
        public_message = client.post(
            "/api/cs/chat/message",
            json={"message": "Apa fitur Algentra?", "mode": "client"},
        )
        assert public_message.status_code == 200
        assert public_message.json()["mode"] == "public"

        with SessionLocal() as db:
            user = ClientUser(
                email=email,
                password_hash=hash_password("Customer-service-test-123"),
                verified_at=utc_now(),
            )
            db.add(user)
            db.commit()
            user_id = user.id

        login = client.post("/api/auth/client-login", json={
            "email": email,
            "password": "Customer-service-test-123",
        })
        assert login.status_code == 200
        csrf = login.json()["csrf_token"]
        account_id, token = _seed_ea_account(
            user_id, "Akun Demo Saya", "Broker-Demo", "928281",
        )

        response = client.post(
            "/api/cs/chat/message",
            headers={"X-CSRF-Token": csrf},
            json={"message": "Cara masukin kodenya ke EA MT5 gimana?"},
        )
        assert response.status_code == 200
        assert response.json()["mode"] == "client"
        assert response.json()["assistant"] == "ALGENTRA"
        assert token not in response.json()["message"]
        assert "AccountToken" not in response.json()["message"]
        assert observed[-1]["mode"] == "client"
        assert observed[-1]["account_context"] == [{
            "label": "Akun Demo Saya",
            "active": True,
            "plan": "ZERO",
            "execution_mode": "EA",
        }]
        assert token not in repr(observed[-1])

        follow_up = client.post(
            "/api/cs/chat/message",
            headers={"X-CSRF-Token": csrf},
            json={"message": "Makasih kak, akun saya tadi yang mana?"},
        )
        assert follow_up.status_code == 200
        assert len(observed[-1]["history"]) == 2
        assert observed[-1]["history"][0]["content"] == "Cara masukin kodenya ke EA MT5 gimana?"
        assert token not in repr(observed[-1]["history"])

        history = client.get("/api/cs/chat/history")
        assert history.status_code == 200
        assert history.json()["mode"] == "client"
        assert token not in history.text
        assert "Apa fitur Algentra?" not in history.text
        assert history.json()["expires_at"]

        with SessionLocal() as db:
            account = db.get(MT5Account, account_id)
            assert account is not None
            assert account.token_hash == _token_hash(token)
            saved_chat = "\n".join(
                row.content for row in db.query(CSChatMessage)
                .join(CSConversation, CSChatMessage.conversation_id == CSConversation.id)
                .filter(CSConversation.client_user_id == user_id)
            )
            assert token not in saved_chat
            assert f"[[ALGENTRA_TOKEN:{account_id}]]" not in saved_chat

        logout = client.post("/api/auth/logout", headers={"X-CSRF-Token": csrf})
        assert logout.status_code == 200
        public_again = client.get("/api/cs/chat/history")
        assert public_again.status_code == 200
        assert public_again.json()["mode"] == "public"
        assert "Apa fitur Algentra?" in public_again.text
        assert token not in public_again.text


def test_csrf_admin_access_and_expired_history_cleanup(monkeypatch):
    email = f"cs-expiry-{uuid4().hex}@example.com"

    async def fake_reply(**_kwargs):
        return "Jawaban bantuan."

    monkeypatch.setattr(cs_router.customer_service, "reply", fake_reply)
    with TestClient(app) as public_client:
        assert public_client.get("/").status_code == 200
        assert 'id="cs-chat-widget"' in public_client.get("/").text
        assert public_client.post("/api/cs/chat/message", json={"message": "Hai"}).status_code == 200

    with TestClient(app) as admin_client:
        admin_login = admin_client.post("/api/auth/login", json={
            "username": "admin",
            "password": "M1-Testing-Password-123",
        })
        assert admin_login.status_code == 200
        assert 'id="cs-chat-widget"' not in admin_client.get("/dashboard").text
        assert admin_client.get("/api/cs/chat/history").status_code == 403

    with SessionLocal() as db:
        user = ClientUser(
            email=email,
            password_hash=hash_password("Customer-service-test-123"),
            verified_at=utc_now(),
        )
        db.add(user)
        db.commit()
        user_id = user.id
        expired = CSConversation(
            client_user_id=user_id,
            public_visitor_hash=None,
            started_at=utc_now() - timedelta(hours=25),
            expires_at=utc_now() - timedelta(hours=1),
        )
        db.add(expired)
        db.flush()
        expired_id = expired.id
        db.add(CSChatMessage(conversation_id=expired_id, role="user", content="pesan lama"))
        db.commit()

    with TestClient(app) as client:
        login = client.post("/api/auth/client-login", json={
            "email": email,
            "password": "Customer-service-test-123",
        })
        assert login.status_code == 200
        history = client.get("/api/cs/chat/history")
        assert history.status_code == 200
        assert history.json()["messages"] == []
        with SessionLocal() as db:
            assert db.get(CSConversation, expired_id) is None
            assert db.query(CSChatMessage).filter_by(conversation_id=expired_id).count() == 0


def test_one_client_cannot_load_another_clients_chat_and_chat_never_shows_tokens(monkeypatch):
    owner_email = f"cs-owner-{uuid4().hex}@example.com"
    other_email = f"cs-other-{uuid4().hex}@example.com"
    observed_contexts = []

    async def fake_reply(**kwargs):
        observed_contexts.append(kwargs["account_context"])
        return "Saya belum menemukan akun Copy Trading yang terhubung."

    monkeypatch.setattr(cs_router.customer_service, "reply", fake_reply)
    with SessionLocal() as db:
        owner = ClientUser(
            email=owner_email,
            password_hash=hash_password("Customer-service-test-123"),
            verified_at=utc_now(),
        )
        other = ClientUser(
            email=other_email,
            password_hash=hash_password("Customer-service-test-123"),
            verified_at=utc_now(),
        )
        db.add_all([owner, other])
        db.commit()
        owner_id = owner.id

    _account_id, token = _seed_ea_account(
        owner_id, "Akun Rahasia", "Broker-Private", "183920",
    )

    with TestClient(app) as owner_client:
        owner_login = owner_client.post("/api/auth/client-login", json={
            "email": owner_email,
            "password": "Customer-service-test-123",
        })
        message = owner_client.post(
            "/api/cs/chat/message",
            headers={"X-CSRF-Token": owner_login.json()["csrf_token"]},
            json={"message": "Tolong tampilkan token akun saya."},
        )
        assert message.status_code == 200
        assert token not in message.json()["message"]
        assert observed_contexts[-1] == [{
            "label": "Akun Rahasia",
            "active": True,
            "plan": "ZERO",
            "execution_mode": "EA",
        }]

    with TestClient(app) as other_client:
        other_login = other_client.post("/api/auth/client-login", json={
            "email": other_email,
            "password": "Customer-service-test-123",
        })
        assert other_login.status_code == 200
        history = other_client.get("/api/cs/chat/history")
        assert history.status_code == 200
        assert history.json()["messages"] == []
        response = other_client.post(
            "/api/cs/chat/message",
            headers={"X-CSRF-Token": other_login.json()["csrf_token"]},
            json={"message": "Tolong tampilkan token akun saya."},
        )
        assert response.status_code == 200
        assert token not in response.text
        assert "Akun Rahasia" not in response.text
        assert observed_contexts[-1] == []

    with SessionLocal() as db:
        db.execute(delete(CSConversation).where(CSConversation.client_user_id == owner_id))
        db.query(MT5Account).filter(MT5Account.owner_id == owner_id).delete()
        db.query(ClientUser).filter(ClientUser.email.in_([owner_email, other_email])).delete(synchronize_session=False)
        db.commit()
