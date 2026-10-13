from datetime import datetime, timezone

import pytest
from fastapi.testclient import TestClient

from app.api.v1 import public as public_api
from app.main import app as application
from app.services.public_service import equity_session_start


@pytest.mark.parametrize(
    ("now", "expected_start"),
    [
        (
            datetime(2026, 10, 8, 23, 59, tzinfo=timezone.utc),
            datetime(2026, 10, 8, 0, 0, tzinfo=timezone.utc),
        ),
        (
            datetime(2026, 10, 9, 0, 0, tzinfo=timezone.utc),
            datetime(2026, 10, 9, 0, 0, tzinfo=timezone.utc),
        ),
        (
            datetime(2026, 10, 9, 0, 1, tzinfo=timezone.utc),
            datetime(2026, 10, 9, 0, 0, tzinfo=timezone.utc),
        ),
    ],
)
def test_equity_session_starts_at_seven_wib(now, expected_start):
    assert equity_session_start(now) == expected_start


def test_public_music_github_file_links_redirect_to_raw_audio(monkeypatch):
    monkeypatch.setattr(
        public_api,
        "get_settings",
        lambda: type(
            "Settings",
            (),
            {"public_music_url": "https://github.com/example/audio/blob/main/assets/theme.mp3"},
        )(),
    )

    response = public_api.site_music()

    assert response.headers["location"] == "https://raw.githubusercontent.com/example/audio/main/assets/theme.mp3"


def test_public_performance_can_skip_equity_history(monkeypatch):
    monkeypatch.setattr(
        public_api,
        "public_performance_payload",
        lambda _db, *, include_equity_curve: {"include_equity_curve": include_equity_curve},
    )

    assert public_api.public_performance(object(), include_equity_curve=False) == {"include_equity_curve": False}


def test_public_performance_endpoint_can_omit_equity_history():
    with TestClient(application) as client:
        responses = [
            client.get(f"{path}?include_equity_curve=false")
            for path in ("/api/public/performance", "/v1/public/performance")
        ]

    assert all(response.status_code == 200 for response in responses)
    assert all("equity_curve" not in response.json() for response in responses)
