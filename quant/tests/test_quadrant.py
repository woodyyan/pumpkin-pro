import numpy as np
import pandas as pd
import pytest

pytest.importorskip("requests", reason="screener.quadrant 依赖 requests")

from screener.quadrant import (
    _classify_a_share_board,
    _compute_daily_metrics,
    _derive_progress_url,
    _latest_cached_close,
    _resolve_end_date,
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


# ── _resolve_end_date ──


def test_resolve_end_date_uses_source_trade_date():
    """传入合法日期应原样返回。"""
    assert _resolve_end_date("2026-06-15") == "2026-06-15"


def test_resolve_end_date_none_defaults_to_today():
    """传入 None 应返回今天。"""
    today = pd.Timestamp.today().strftime("%Y-%m-%d")
    assert _resolve_end_date(None) == today


def test_resolve_end_date_empty_string_defaults_to_today():
    """传入空字符串应返回今天。"""
    today = pd.Timestamp.today().strftime("%Y-%m-%d")
    assert _resolve_end_date("") == today


def test_resolve_end_date_invalid_falls_back_to_today():
    """传入无效日期应回退到今天并发出 warning。"""
    today = pd.Timestamp.today().strftime("%Y-%m-%d")
    result = _resolve_end_date("not-a-date")
    assert result == today


def test_resolve_end_date_normalizes_format():
    """传入 Timestamp 可解析的格式应统一输出 YYYY-MM-DD。"""
    assert _resolve_end_date("2026-01-05") == "2026-01-05"

# ── Data Source Gateway integration ──

class StubQuadrantDataSourceManager:
    def __init__(self):
        self.daily_requests = []
        self.index_requests = []

    def fetch_daily_bars(self, **kwargs):
        from data_sources.models import DataSourceResponse, DailyBar

        self.daily_requests.append(kwargs)
        market = kwargs["market"]
        symbol = kwargs["symbol"]
        return DataSourceResponse(
            ok=True,
            capability="daily_bars",
            market=market,
            symbol=symbol,
            data=[
                DailyBar(symbol=symbol, market=market, trade_date="2026-07-09", open=10, close=10, high=11, low=9, volume=100),
                DailyBar(symbol=symbol, market=market, trade_date="2026-07-10", open=10, close=12, high=13, low=9, volume=120),
            ],
            used_sources=["stub"],
        )

    def fetch_index_bars(self, **kwargs):
        from data_sources.models import DataSourceResponse, DailyBar

        self.index_requests.append(kwargs)
        market = kwargs["market"]
        symbol = kwargs["symbol"]
        return DataSourceResponse(
            ok=True,
            capability="index_bars",
            market=market,
            symbol=symbol,
            data=[
                DailyBar(symbol=symbol, market=market, trade_date="2026-05-01", open=100, close=100, high=101, low=99),
                DailyBar(symbol=symbol, market=market, trade_date="2026-07-10", open=110, close=120, high=121, low=109),
            ],
            used_sources=["stub"],
        )


def test_quadrant_a_share_daily_bars_use_data_source_gateway(monkeypatch):
    import screener.quadrant as quadrant

    original = quadrant._DATA_SOURCE_MANAGER
    stub = StubQuadrantDataSourceManager()
    quadrant.set_data_source_manager(stub)
    try:
        bars = quadrant._fetch_daily_bars("000001", days=90, source_trade_date="2026-07-10")
    finally:
        quadrant.set_data_source_manager(original)

    assert bars[-1]["date"] == "2026-07-10"
    assert stub.daily_requests == [{
        "symbol": "000001",
        "market": "ASHARE",
        "target_trade_date": "2026-07-10",
        "lookback_days": 90,
        "adjust": "qfq",
    }]


def test_quadrant_hk_daily_bars_use_data_source_gateway(monkeypatch):
    import screener.quadrant as quadrant

    original = quadrant._DATA_SOURCE_MANAGER
    stub = StubQuadrantDataSourceManager()
    quadrant.set_data_source_manager(stub)
    try:
        bars = quadrant._fetch_daily_bars_hk("00700", days=30, source_trade_date="2026-07-10")
    finally:
        quadrant.set_data_source_manager(original)

    assert bars[0]["provider"] == ""
    assert stub.daily_requests[0]["market"] == "HKEX"
    assert stub.daily_requests[0]["symbol"] == "00700"
    assert stub.daily_requests[0]["target_trade_date"] == "2026-07-10"


def test_quadrant_benchmark_uses_data_source_gateway():
    import screener.quadrant as quadrant

    original = quadrant._DATA_SOURCE_MANAGER
    stub = StubQuadrantDataSourceManager()
    quadrant.set_data_source_manager(stub)
    try:
        ret = quadrant._fetch_benchmark_60d_return(source_trade_date="2026-07-10")
    finally:
        quadrant.set_data_source_manager(original)

    assert ret == pytest.approx(20.0)
    assert stub.index_requests[0]["symbol"] == "000001"
    assert stub.index_requests[0]["market"] == "ASHARE"
    assert stub.index_requests[0]["target_trade_date"] == "2026-07-10"


def test_quadrant_hsi_benchmark_uses_data_source_gateway():
    import screener.quadrant as quadrant

    original = quadrant._DATA_SOURCE_MANAGER
    stub = StubQuadrantDataSourceManager()
    quadrant.set_data_source_manager(stub)
    try:
        ret = quadrant._fetch_hsi_60d_return(source_trade_date="2026-07-10")
    finally:
        quadrant.set_data_source_manager(original)

    assert ret == pytest.approx(20.0)
    assert stub.index_requests[0]["symbol"] == "HSI"
    assert stub.index_requests[0]["market"] == "HKEX"
