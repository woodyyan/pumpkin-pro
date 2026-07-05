import numpy as np
import pandas as pd
import pytest

pytest.importorskip("requests", reason="screener.quadrant 依赖 requests")

from screener.quadrant import (
    _classify_a_share_board,
    _compute_daily_metrics,
    _derive_progress_url,
    _latest_cached_close,
    _safe_momentum,
)


def test_classify_a_share_board():
    assert _classify_a_share_board("600519") == "MAIN"
    assert _classify_a_share_board("000001") == "MAIN"
    assert _classify_a_share_board("002415") == "MAIN"
    assert _classify_a_share_board("300750") == "CHINEXT"
    assert _classify_a_share_board("301123") == "CHINEXT"
    assert _classify_a_share_board("688981") == "STAR"
    assert _classify_a_share_board("689009") == "STAR"
    assert _classify_a_share_board("430047") == "OTHER"


def test_safe_momentum_prefers_stable_stock():
    volatile = pd.Series([20.0], index=["A"])
    stable = pd.Series([15.0], index=["B"])
    returns = pd.concat([volatile, stable])
    volatility = pd.Series([0.20, 0.05], index=["A", "B"])

    momentum = _safe_momentum(returns, volatility)

    assert momentum["B"] > momentum["A"]


def test_safe_momentum_uses_floor_for_zero_or_missing_volatility():
    returns = pd.Series([12.5, 8.0], index=["zero", "nan"])
    volatility = pd.Series([0.0, np.nan], index=["zero", "nan"])

    momentum = _safe_momentum(returns, volatility, floor=0.005)

    assert momentum["zero"] == pytest.approx(2500.0)
    assert momentum["nan"] == pytest.approx(1600.0)


def test_compute_daily_metrics_includes_std_60d():
    close = pd.Series([100 + i * 0.8 + ((-1) ** i) * 0.5 for i in range(70)], dtype=float)
    volume = pd.Series([1_000_000 + i * 10_000 for i in range(70)], dtype=float)
    daily_df = pd.DataFrame(
        {
            "date": pd.date_range("2026-01-01", periods=70, freq="D"),
            "close": close,
            "volume": volume,
        }
    )

    metrics = _compute_daily_metrics(daily_df)
    expected_std_60d = float(close.pct_change().dropna().tail(60).std())

    assert metrics["std_20d"] > 0
    assert metrics["std_60d"] == pytest.approx(expected_std_60d)
    assert metrics["avg_amount_5d"] > 0


def test_compute_daily_metrics_std_60d_falls_back_to_full_window_when_under_60_days():
    close = pd.Series([50 + i * 0.6 + (i % 3) * 0.2 for i in range(35)], dtype=float)
    daily_df = pd.DataFrame(
        {
            "date": pd.date_range("2026-02-01", periods=35, freq="D"),
            "close": close,
            "volume": pd.Series([500_000 + i * 5000 for i in range(35)], dtype=float),
        }
    )

    metrics = _compute_daily_metrics(daily_df)
    expected_std = float(close.pct_change().dropna().std())

    assert metrics["std_60d"] == pytest.approx(expected_std)


class StubDailyBarCache:
    def __init__(self, bars):
        self.bars = bars

    def get_stock_bars(self, code):
        return self.bars.get(code)


def test_latest_cached_close_uses_latest_positive_close():
    cache = StubDailyBarCache({
        "600519": [
            {"date": "2026-04-16", "close": 101.2},
            {"date": "2026-04-17", "close": 0},
            {"date": "2026-04-20", "close": 103.4},
        ]
    })

    close_price, trade_date = _latest_cached_close(cache, "600519")

    assert close_price == pytest.approx(103.4)
    assert trade_date == "2026-04-20"


def test_latest_cached_close_skips_missing_or_bad_prices():
    cache = StubDailyBarCache({
        "00700": [
            {"date": "2026-04-18", "close": 412.0},
            {"date": "2026-04-21", "close": None},
            {"date": "2026-04-22", "close": "bad"},
        ]
    })

    close_price, trade_date = _latest_cached_close(cache, "00700")

    assert close_price == pytest.approx(412.0)
    assert trade_date == "2026-04-18"


def test_latest_cached_close_returns_empty_when_unavailable():
    cache = StubDailyBarCache({"000001": [{"date": "2026-04-20", "close": 0}]})

    close_price, trade_date = _latest_cached_close(cache, "000001")

    assert close_price == 0
    assert trade_date == ""


# ── _derive_progress_url ──


def test_derive_progress_url_plain():
    """无 query string 的 callback_url 应正确推导出 progress URL。"""
    url = _derive_progress_url("http://backend:8080/api/quadrant/bulk-save")
    assert url == "http://backend:8080/api/quadrant/progress"


def test_derive_progress_url_with_query_string():
    """带 query string 的 callback_url 应保留 query 并替换 path 末段。"""
    url = _derive_progress_url(
        "http://backend:8080/api/quadrant/bulk-save?source_trade_date=2026-07-02"
    )
    assert url == "http://backend:8080/api/quadrant/progress?source_trade_date=2026-07-02"


def test_derive_progress_url_trailing_slash():
    """尾部斜杠不应影响推导结果。"""
    url = _derive_progress_url("http://backend:8080/api/quadrant/bulk-save/")
    assert url == "http://backend:8080/api/quadrant/progress"


def test_derive_progress_url_trailing_slash_with_query():
    """尾部斜杠 + query string 的组合应正确处理。"""
    url = _derive_progress_url(
        "http://backend:8080/api/quadrant/bulk-save/?source_trade_date=2026-07-02"
    )
    assert url == "http://backend:8080/api/quadrant/progress?source_trade_date=2026-07-02"


def test_derive_progress_url_none():
    """传入 None 应返回 None。"""
    assert _derive_progress_url(None) is None


def test_derive_progress_url_empty():
    """传入空字符串应返回 None。"""
    assert _derive_progress_url("") is None


def test_derive_progress_url_preserves_multiple_params():
    """多个 query 参数都应保留。"""
    url = _derive_progress_url(
        "http://backend:8080/api/quadrant/bulk-save?source_trade_date=2026-07-02&force_full=true"
    )
    assert url == "http://backend:8080/api/quadrant/progress?source_trade_date=2026-07-02&force_full=true"
