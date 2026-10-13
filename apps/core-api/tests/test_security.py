from fastapi import FastAPI
from fastapi.testclient import TestClient
from sqlalchemy.exc import SQLAlchemyError

import app.main as main_module
from app.main import app as application
from app.core.security import RateLimitMiddleware


def test_rate_limit_returns_429_after_configured_budget(monkeypatch):
    monkeypatch.setattr(RateLimitMiddleware, "_limit", staticmethod(lambda _path: (2, 60)))
    app = FastAPI()
    app.add_middleware(RateLimitMiddleware)

    @app.get("/limited")
    def limited():
        return {"status": "ok"}

    with TestClient(app) as client:
        assert client.get("/limited").status_code == 200
        assert client.get("/limited").status_code == 200
        response = client.get("/limited")
        assert response.status_code == 429
        assert response.headers["retry-after"] == "60"


def test_application_security_headers_and_api_docs_are_disabled():
    with TestClient(application) as client:
        response = client.get("/")
        assert response.status_code == 200
        csp = response.headers["content-security-policy"]
        assert "script-src 'self' 'nonce-" in csp
        assert "media-src 'self'" in csp
        assert "'unsafe-inline'" not in csp
        nonce = csp.split("'nonce-", 1)[1].split("'", 1)[0]
        assert f'nonce="{nonce}"' in response.text
        assert f'<style nonce="{nonce}"' in response.text
        assert response.headers["x-frame-options"] == "DENY"
        assert response.headers["x-content-type-options"] == "nosniff"
        assert response.headers["referrer-policy"] == "strict-origin-when-cross-origin"
        assert "geolocation=()" in response.headers["permissions-policy"]
        assert client.get("/docs").status_code == 404
        assert client.get("/redoc").status_code == 404
        assert client.get("/openapi.json").status_code == 404
        client.cookies.set("theme", "light")
        themed_page = client.get("/")
        assert 'data-theme="light"' in themed_page.text
        assert 'id="theme-toggle"' in themed_page.text


def test_readiness_reports_database_connectivity_without_connection_details(monkeypatch):
    with TestClient(application) as client:
        ready = client.get("/ready")
        assert ready.status_code == 200
        assert ready.json() == {"status": "ready", "checks": {"database": "ok"}}

        class UnavailableEngine:
            def connect(self):
                raise SQLAlchemyError("private database connection details")

        monkeypatch.setattr(main_module, "engine", UnavailableEngine())
        unavailable = client.get("/ready")
        assert unavailable.status_code == 503
        assert unavailable.json() == {"status": "not_ready", "checks": {"database": "unavailable"}}
        assert "private database connection details" not in unavailable.text
