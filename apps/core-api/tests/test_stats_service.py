from types import SimpleNamespace

import pytest

from datetime import datetime, timezone

from app.services.stats_service import calculate_daily_loss, calculate_trade_stats, get_dashboard_stats


@pytest.mark.parametrize(
    ("results", "wins", "losses", "winrate"),
    [
        (["WIN", "LOSS", "WIN"], 2, 1, 66.67),
        (["WIN", "WIN"], 2, 0, 100.0),
        (["LOSS", "LOSS"], 0, 2, 0.0),
        (["WIN", "LOSS", "BE", None], 1, 1, 50.0),
        ([], 0, 0, 0.0),
    ],
)
def test_trade_stats_win_loss_and_winrate(results, wins, losses, winrate):
    trades = [SimpleNamespace(result=result, profit=10.0 if result == "WIN" else -4.0) for result in results]

    stats = calculate_trade_stats(trades)

    assert stats["wins"] == wins
    assert stats["losses"] == losses
    assert stats["winrate"] == winrate


def test_winrate_excludes_break_even_and_open_trades():
    trades = [
        SimpleNamespace(result="WIN", profit=12),
        SimpleNamespace(result="LOSS", profit=-5),
        SimpleNamespace(result="BE", profit=0),
        SimpleNamespace(result=None, profit=0),
    ]

    stats = calculate_trade_stats(trades)

    assert stats["winrate"] == pytest.approx(50.0)
    assert stats["total_profit"] == pytest.approx(7.0)


def test_dashboard_signal_categories_cover_expired_and_report_unknown_statuses(monkeypatch):
    from app.services import stats_service

    class FakeStatisticsRepository:
        def __init__(self, _db):
            pass

        def dashboard_data(self):
            return ["EXECUTED", "PENDING", "CLAIMED", "PROCESSING", "REJECTED", "FAILED", "IGNORED", "EXPIRED", "MYSTERY"], [], 0, ["MYSTERY"]

    monkeypatch.setattr(stats_service, "StatisticsRepository", FakeStatisticsRepository)
    result = get_dashboard_stats(object())

    categorized = (
        result["signals_executed"]
        + result["signals_pending"]
        + result["signals_processing"]
        + result["signals_rejected"]
        + result["signals_ignored"]
        + result["signals_expired"]
    )
    assert result["signals_total"] == 9
    assert categorized == 8
    assert result["signals_processing"] == 1
    assert result["signals_expired"] == 1
    assert result["signal_status_unknown"] is True
    assert result["signal_status_unknown_values"] == ["MYSTERY"]


def test_dashboard_counts_final_trade_results_as_executed_signals(monkeypatch):
    from app.services import stats_service

    class FakeStatisticsRepository:
        def __init__(self, _db):
            pass

        def dashboard_data(self):
            return ["EXECUTED", "WIN", "LOSS", "BE"], [], 0, []

    monkeypatch.setattr(stats_service, "StatisticsRepository", FakeStatisticsRepository)
    result = get_dashboard_stats(object())

    assert result["signals_total"] == 4
    assert result["signals_executed"] == 4
    assert result["signal_status_unknown"] is False


def test_daily_loss_counts_only_closed_negative_trades_on_or_after_day_start():
    day_start = datetime(2026, 10, 13, 17, 0)  # 00.00 WIB, stored UTC naive
    trades = [
        SimpleNamespace(profit=-45.25, closed_at=datetime(2026, 10, 13, 18, 0, tzinfo=timezone.utc)),
        SimpleNamespace(profit=80.0, closed_at=datetime(2026, 10, 13, 19, 0, tzinfo=timezone.utc)),
        SimpleNamespace(profit=-30.0, closed_at=datetime(2026, 10, 13, 16, 59, tzinfo=timezone.utc)),
        SimpleNamespace(profit=-20.0, closed_at=None),
    ]

    assert calculate_daily_loss(trades, day_start_utc=day_start) == 45.25
