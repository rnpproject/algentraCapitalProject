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
        response = client.get("/health")
        assert response.status_code == 200
        csp = response.headers["content-security-policy"]
        assert "script-src 'self' 'nonce-" in csp
        assert "media-src 'self'" in csp
        assert "'unsafe-inline'" not in csp
        assert response.headers["x-frame-options"] == "DENY"
        assert response.headers["x-content-type-options"] == "nosniff"
        assert response.headers["referrer-policy"] == "strict-origin-when-cross-origin"
        assert "geolocation=()" in response.headers["permissions-policy"]
        assert client.get("/docs").status_code == 404
        assert client.get("/redoc").status_code == 404
        assert client.get("/openapi.json").status_code == 404
        # Web sudah pindah ke Vercel; path tak dikenal mengembalikan JSON 404.
        missing = client.get("/tidak-ada")
        assert missing.status_code == 404
        assert missing.json() == {"detail": "Not Found"}


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
