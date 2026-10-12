from datetime import datetime, timezone
import math

from sqlalchemy.orm import Session

from ..core.time_utils import WIB, as_utc, wib_day_start_utc_naive
from ..repositories.risk import RiskRepository
from ..repositories.settings import SettingsRepository


DEFAULT_CONTROLS = {
    "kill_switch": False,
    "max_market_deviation_pct": 5.0,
}


def setting_value(db: Session, key: str, default):
    return SettingsRepository(db).get_json(key, default)


def risk_state(db: Session, now: datetime | None = None) -> dict[str, object]:
    controls = {key: setting_value(db, key, value) for key, value in DEFAULT_CONTROLS.items()}
    now = now or datetime.now(timezone.utc)
    now = as_utc(now)
    start_of_day = wib_day_start_utc_naive(now.astimezone(WIB).date())
    daily_profit, open_trades = RiskRepository(db).daily_profit_and_open_trade_count(start_of_day)

    raw_deviation = controls["max_market_deviation_pct"]
    max_deviation = (
        float(raw_deviation)
        if isinstance(raw_deviation, (int, float))
        and not isinstance(raw_deviation, bool)
        and math.isfinite(raw_deviation)
        and 0.1 <= raw_deviation <= 100
        else 5.0
    )
    kill_switch = controls["kill_switch"] is True
    daily_loss = max(0.0, -daily_profit)

    controls.update(
        {
            "kill_switch": kill_switch,
            "daily_profit": round(daily_profit, 2),
            "daily_loss": round(daily_loss, 2),
            "open_trades": open_trades,
            "max_market_deviation_pct": max_deviation,
            "trading_paused": kill_switch,
        }
    )
    return controls
