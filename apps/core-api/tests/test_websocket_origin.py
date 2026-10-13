from app.main import websocket_origin_allowed


def test_websocket_origin_accepts_same_host_and_explicit_proxy_origin_only():
    assert websocket_origin_allowed(
        "http://127.0.0.1:8000",
        "127.0.0.1:8000",
        "",
    )
    assert websocket_origin_allowed(
        "http://127.0.0.1:3000",
        "127.0.0.1:8000",
        "http://127.0.0.1:3000,http://localhost:3000",
    )
    assert not websocket_origin_allowed(
        "https://evil.example",
        "127.0.0.1:8000",
        "http://127.0.0.1:3000,http://localhost:3000",
    )
