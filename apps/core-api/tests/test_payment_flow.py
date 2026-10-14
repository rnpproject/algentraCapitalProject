from types import SimpleNamespace

from fastapi.testclient import TestClient
from pydantic import SecretStr

from app.api.v1 import payments as payments_api
from app.auth import hash_password
from app.database import SessionLocal
from app.main import app
from app.models import AuthSession, ClientUser, MT5Account, PaymentOrder, utc_now
from app.services import payment_service
from app.services.payment_review_service import review_payment_order


def test_manual_payment_review_provisions_managed_account_only_after_approval(monkeypatch, tmp_path):
    email = "manual-payment-flow@example.com"
    password = "Manual-Payment-Client-123"
    worker_key = "m5-managed-copy-worker-key-at-least-32-characters"
    monkeypatch.setattr(
        payment_service,
        "payment_proof_path",
        lambda filename: tmp_path / filename,
    )

    async def notify_order(order_id):
        return order_id > 0

    monkeypatch.setattr(payment_service.payment_review_bot, "notify_order", notify_order)
    with TestClient(app) as client:
        with SessionLocal() as db:
            db.add(
                ClientUser(
                    email=email,
                    password_hash=hash_password(password),
                    verified_at=utc_now(),
                )
            )
            db.commit()

        login = client.post("/api/auth/client-login", json={"email": email, "password": password})
        assert login.status_code == 200
        csrf = {"X-CSRF-Token": login.json()["csrf_token"]}
        order_response = client.post(
            "/api/payments/orders",
            headers=csrf,
            json={
                "label": "Managed Demo",
                "server": "Broker-Demo",
                "login": "12345678",
                "plan": "PRO",
                "execution_mode": "MANAGED",
            },
        )
        assert order_response.status_code == 503

        monkeypatch.setattr(
            payments_api,
            "get_settings",
            lambda: SimpleNamespace(copier_worker_api_key=SecretStr(worker_key)),
        )
        order_response = client.post(
            "/api/payments/orders",
            headers=csrf,
            json={
                "label": "Managed Demo",
                "server": "Broker-Demo",
                "login": "12345678",
                "plan": "PRO",
                "execution_mode": "MANAGED",
            },
        )
        assert order_response.status_code == 200
        order = order_response.json()
        assert order["status"] == "PENDING_PAYMENT"
        assert order["total_idr"] == 200_000
        assert order["email"] == email

        page = client.get("/transactions")
        # Halaman /transactions sudah pindah ke Vercel; Core API hanya melayani API JSON.
        assert page.status_code == 404
        assert page.json() == {"detail": "Not Found"}

        proof = b"\x89PNG\r\n\x1a\n" + b"manual-payment-proof"
        proof_response = client.post(
            f"/api/payments/orders/{order['id']}/proof",
            headers=csrf,
            files={"file": ("receipt.png", proof, "image/png")},
        )
        assert proof_response.status_code == 200
        assert proof_response.json()["status"] == "PAYMENT_SUBMITTED"
        assert proof_response.json()["notification_sent"] is True
        assert client.get(f"/api/payments/orders/{order['id']}/proof").content == proof

        with SessionLocal() as db:
            submitted = db.get(PaymentOrder, order["id"])
            assert submitted is not None
            assert submitted.status == "PAYMENT_SUBMITTED"
            assert submitted.account_id is None
        with SessionLocal() as db:
            status, account_id = review_payment_order(db, order["id"], "accept")
            assert status == "APPROVED"
            assert account_id is not None
            account = db.get(MT5Account, account_id)
            assert account is not None
            assert account.execution_mode == "MANAGED"
            assert account.login == "12345678"
            account_owner_id = account.owner_id

        account_list = client.get("/api/mt5/accounts")
        assert account_list.status_code == 200
        assert account_list.json()[0]["execution_mode"] == "MANAGED"
        assert account_list.json()[0]["managed_copy"]["configured"] is False
        assert "broker_password" not in account_list.text

        with SessionLocal() as db:
            user = db.query(ClientUser).filter_by(email=email).one_or_none()
            if user is not None:
                db.query(AuthSession).filter_by(user_type="client", user_id=str(user.id)).delete()
            db.query(PaymentOrder).filter_by(email=email).delete()
            db.query(MT5Account).filter_by(owner_id=account_owner_id).delete()
            db.query(ClientUser).filter_by(email=email).delete()
            db.commit()


def test_payment_proof_requires_supported_image_and_pending_order(monkeypatch, tmp_path):
    email = "invalid-payment-proof@example.com"
    password = "Invalid-Payment-Proof-123"
    monkeypatch.setattr(
        payment_service,
        "payment_proof_path",
        lambda filename: tmp_path / filename,
    )
    with TestClient(app) as client:
        with SessionLocal() as db:
            db.add(
                ClientUser(
                    email=email,
                    password_hash=hash_password(password),
                    verified_at=utc_now(),
                )
            )
            db.commit()
        login = client.post("/api/auth/client-login", json={"email": email, "password": password})
        csrf = {"X-CSRF-Token": login.json()["csrf_token"]}
        order = client.post(
            "/api/payments/orders",
            headers=csrf,
            json={
                "label": "EA Demo",
                "server": "Broker-Demo",
                "login": "87654321",
                "plan": "ZERO",
                "execution_mode": "EA",
            },
        ).json()
        invalid = client.post(
            f"/api/payments/orders/{order['id']}/proof",
            headers=csrf,
            files={"file": ("receipt.png", b"not an image", "image/png")},
        )
        assert invalid.status_code == 409
        assert "JPG, PNG, atau WebP" in invalid.json()["detail"]
        assert client.get("/api/payments/orders").json()[0]["status"] == "PENDING_PAYMENT"

        with SessionLocal() as db:
            user = db.query(ClientUser).filter_by(email=email).one_or_none()
            if user is not None:
                db.query(AuthSession).filter_by(user_type="client", user_id=str(user.id)).delete()
            db.query(PaymentOrder).filter_by(email=email).delete()
            db.query(ClientUser).filter_by(email=email).delete()
            db.commit()
