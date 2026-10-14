"""Public-page read models and safe contact configuration."""

import json
import math
import re
from datetime import datetime, time, timedelta, timezone

from sqlalchemy.orm import Session

from ..models import (
    MT5MasterEquityCandle,
    utc_now,
)
from ..core.time_utils import WIB, as_utc, wib_iso
from ..repositories.public_performance import PublicPerformanceRepository
from ..state import master_snapshot_state


MARKET_SYMBOL_PATTERN = re.compile(r"^[A-Za-z0-9._-]{1,32}$")


def equity_session_start(now: datetime) -> datetime:
    wib_now = as_utc(now).astimezone(WIB)
    session_date = wib_now.date() if wib_now.hour >= 7 else wib_now.date() - timedelta(days=1)
    return datetime.combine(session_date, time(hour=7), tzinfo=WIB).astimezone(timezone.utc)


def public_performance_payload(db: Session, *, include_equity_curve: bool = True) -> dict[str, object]:
    repository = PublicPerformanceRepository(db)
    registered_clients = repository.registered_client_count()
    snapshot = master_snapshot_state.current()
    master_account = snapshot.account if snapshot is not None else None
    now = utc_now()
    master_online = bool(
        master_account
        and snapshot
        and now - as_utc(snapshot.observed_at) <= timedelta(seconds=20)
        and now - as_utc(master_account.observed_at) <= timedelta(seconds=30)
    )
    market_quotes: list[dict[str, object]] = []
    if snapshot is not None and snapshot.market_quotes_json is not None:
        try:
            decoded_quotes = json.loads(snapshot.market_quotes_json)
            if isinstance(decoded_quotes, list):
                for quote in decoded_quotes:
                    if not isinstance(quote, dict):
                        continue
                    try:
                        symbol = quote["symbol"]
                        bid = float(quote["bid"])
                        ask = float(quote["ask"])
                        if (
                            not isinstance(symbol, str)
                            or not MARKET_SYMBOL_PATTERN.fullmatch(symbol)
                            or not math.isfinite(bid)
                            or not math.isfinite(ask)
                            or bid <= 0
                            or ask < bid
                        ):
                            continue
                        tick_time = datetime.fromtimestamp(int(quote["time_msc"]) / 1000, timezone.utc)
                        market_quotes.append(
                            {
                                "symbol": symbol,
                                "bid": bid,
                                "ask": ask,
                                "updated_at": wib_iso(tick_time),
                            }
                        )
                    except (KeyError, TypeError, ValueError, OverflowError, OSError):
                        continue
        except (TypeError, json.JSONDecodeError):
            market_quotes = []
    market_quotes_online = bool(
        master_online
        and snapshot
        and snapshot.market_observed_at
        and now - as_utc(snapshot.market_observed_at) <= timedelta(seconds=30)
    )
    equity_curve: list[dict[str, object]] = []
    equity_candles: list[MT5MasterEquityCandle] = []
    if include_equity_curve:
        session_start = equity_session_start(now)
        latest_equity_account_key = repository.latest_equity_account_key()
        equity_candles = (
            repository.equity_candles(latest_equity_account_key, session_start) if latest_equity_account_key else []
        )
        equity_curve = [
            {
                "hour": wib_iso(candle.minute_start),
                "equity": round(candle.close_equity, 2),
                "currency": candle.currency,
            }
            for candle in equity_candles
        ]
    payload: dict[str, object] = {
        "master_account": {
            "available": master_account is not None,
            "online": master_online,
            "account_key": latest_equity_account_key if include_equity_curve else None,
            "balance": master_account.balance if master_account else None,
            "equity": master_account.equity if master_account else None,
            "floating_profit": master_account.floating_profit if master_account else None,
            "currency": master_account.currency if master_account else None,
            "trade_mode": master_account.trade_mode if master_account else None,
            "updated_at": wib_iso(master_account.observed_at) if master_account else None,
        },
        "market_quotes": market_quotes,
        "market_quotes_online": market_quotes_online,
        "market_quotes_reported_at": wib_iso(snapshot.market_observed_at)
        if snapshot is not None and snapshot.market_observed_at
        else None,
        "registered_clients": registered_clients,
    }
    if include_equity_curve:
        current_minute = now.replace(second=0, microsecond=0)
        payload["equity_curve"] = equity_curve
        payload["equity_candles"] = [
            {
                "minute": wib_iso(candle.minute_start),
                "open": round(candle.open_equity, 2),
                "high": round(candle.high_equity, 2),
                "low": round(candle.low_equity, 2),
                "close": round(candle.close_equity, 2),
                "currency": candle.currency,
                "closed": as_utc(candle.minute_start) < current_minute,
                "observed_at": wib_iso(candle.observed_at),
            }
            for candle in equity_candles
        ]
    return payload
