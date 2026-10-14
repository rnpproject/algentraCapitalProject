"""Tests for the auth session status and public contact endpoints."""

from fastapi.testclient import TestClient

from app.main import app as application


def test_auth_session_without_login_reports_logged_out():
    with TestClient(application) as client:
        response = client.get("/v1/auth/session")

    assert response.status_code == 200
    assert response.json() == {"admin": False, "client": False}


def test_auth_session_available_on_legacy_api_prefix():
    with TestClient(application) as client:
        response = client.get("/api/auth/session")

    assert response.status_code == 200
    assert response.json() == {"admin": False, "client": False}


def test_public_contact_returns_expected_shape():
    with TestClient(application) as client:
        response = client.get("/v1/public/contact")

    assert response.status_code == 200
    payload = response.json()
    for key in ("person", "whatsapp", "email", "socials"):
        assert key in payload
    assert isinstance(payload["socials"], list)
