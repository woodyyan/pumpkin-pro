"""Backtest 在线行情适配层测试。"""

from datetime import datetime

import pytest

from data.scripts.online_market_data import (
    detect_market,
    fetch_online_bars,
    _source_label,
)
from data_sources.manager import DataSourceManager
from data_sources.models import DailyBar, Market


class StubProvider:
    def __init__(self, rows=None, exc=None):
        self.rows = rows or []
        self.exc = exc
        self.calls = 0

    def fetch(self, request):
        self.calls += 1
        if self.exc:
            raise self.exc
        return self.rows


def _bar(date="2026-07-10", market=Market.HKEX, provider="tencent"):
    return DailyBar(
        symbol="00700",
        market=market,
        trade_date=date,
        open=400.0,
        close=410.0,
        high=415.0,
        low=398.0,
        volume=1000.0,
        provider=provider,
    )


# ---------------------------------------------------------------------------
# detect_market
# ---------------------------------------------------------------------------
def test_detect_market_hk_five_digits():
    assert detect_market("00700") == Market.HKEX


def test_detect_market_a_share_six_digits():
    assert detect_market("600519") == Market.ASHARE


@pytest.mark.parametrize("bad", ["", "70", "AAPL", "1234567", "abc12"])
def test_detect_market_invalid_raises(bad):
    with pytest.raises(ValueError):
        detect_market(bad)


# ---------------------------------------------------------------------------
# _source_label
# ---------------------------------------------------------------------------
def test_source_label_known_combo():
    assert _source_label("tencent", Market.HKEX) == "腾讯财经-港股 (Tencent HK)"
    assert _source_label("baostock", Market.ASHARE) == "BaoStock-A股 (BaoStock)"


def test_source_label_unknown_falls_back_to_provider_name():
    assert _source_label("mystery", Market.HKEX) == "mystery"
    assert _source_label("", Market.HKEX) == "未知数据源"


# ---------------------------------------------------------------------------
# fetch_online_bars —— 正常路径
# ---------------------------------------------------------------------------
def test_fetch_online_bars_hk_success_returns_df_and_source():
    manager = DataSourceManager(providers={
        "tencent": StubProvider(rows=[_bar(provider="tencent")]),
        "eastmoney": StubProvider(exc=RuntimeError("should not reach")),
        "akshare": StubProvider(exc=RuntimeError("should not reach")),
    })

    df, source_name = fetch_online_bars(
        "00700", datetime(2026, 7, 1), datetime(2026, 7, 10), manager
    )

    assert source_name == "腾讯财经-港股 (Tencent HK)"
    # 关键列存在且能被 DataLoader 识别
    for col in ["date", "open", "high", "low", "close", "volume"]:
        assert col in df.columns
    assert len(df) == 1
    assert df.iloc[0]["close"] == 410.0


def test_fetch_online_bars_hk_falls_back_to_eastmoney():
    """港股主源 tencent 失败时应降级到 eastmoney 并标注对应源名。"""
    manager = DataSourceManager(providers={
        "tencent": StubProvider(exc=RuntimeError("tencent down")),
        "eastmoney": StubProvider(rows=[_bar(provider="eastmoney")]),
        "akshare": StubProvider(exc=RuntimeError("akshare down")),
    })

    df, source_name = fetch_online_bars(
        "00700", datetime(2026, 7, 1), datetime(2026, 7, 10), manager
    )

    assert source_name == "东方财富-港股 (EastMoney HK)"
    assert len(df) == 1


# ---------------------------------------------------------------------------
# fetch_online_bars —— 边界与异常
# ---------------------------------------------------------------------------
def test_fetch_online_bars_empty_ticker_raises():
    manager = DataSourceManager(providers={"tencent": StubProvider()})
    with pytest.raises(ValueError):
        fetch_online_bars("", datetime(2026, 7, 1), datetime(2026, 7, 10), manager)


def test_fetch_online_bars_all_sources_fail_raises_readable_error():
    manager = DataSourceManager(providers={
        "tencent": StubProvider(exc=RuntimeError("tencent RemoteDisconnected")),
        "eastmoney": StubProvider(exc=RuntimeError("eastmoney RemoteDisconnected")),
        "akshare": StubProvider(exc=RuntimeError("akshare down")),
    })

    with pytest.raises(RuntimeError) as excinfo:
        fetch_online_bars(
            "00700", datetime(2026, 7, 1), datetime(2026, 7, 10), manager
        )

    msg = str(excinfo.value)
    assert "所有数据源均连接失败" in msg
    # 错误详情应聚合各 provider 的失败原因
    assert "tencent" in msg and "eastmoney" in msg


def test_fetch_online_bars_a_share_uses_a_share_chain():
    manager = DataSourceManager(providers={
        "baostock": StubProvider(rows=[_bar(market=Market.ASHARE, provider="baostock")]),
        "tencent": StubProvider(exc=RuntimeError("should not reach")),
        "eastmoney": StubProvider(exc=RuntimeError("should not reach")),
        "akshare": StubProvider(exc=RuntimeError("should not reach")),
    })

    df, source_name = fetch_online_bars(
        "600519", datetime(2026, 7, 1), datetime(2026, 7, 10), manager
    )

    assert source_name == "BaoStock-A股 (BaoStock)"
    assert len(df) == 1
