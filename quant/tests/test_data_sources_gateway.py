import pytest
import requests

from data_sources.errors import TradeDateMismatchError, ValidationError
from data_sources.manager import DataSourceManager
from data_sources.models import Capability, DataSourceRequest, DailyBar, Market
from data_sources.policy import get_policy
from data_sources.registry import SourceRegistry
from data_sources.validators import validate_daily_bars


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


def bar(date="2026-07-10", provider="stub"):
    return DailyBar(
        symbol="000001",
        market=Market.ASHARE,
        trade_date=date,
        open=10,
        close=11,
        high=12,
        low=9,
        volume=100,
        provider=provider,
    )


def test_policy_is_code_constant_for_daily_and_index_bars():
    assert get_policy(Capability.DAILY_BARS, Market.ASHARE).providers == ["tencent", "eastmoney", "akshare"]
    assert get_policy(Capability.INDEX_BARS, Market.HKEX).providers == ["tencent", "eastmoney", "akshare"]
    assert get_policy(Capability.DAILY_BARS, Market.ASHARE).require_exact_trade_date is True
    assert get_policy(Capability.COMPANY_PROFILE, Market.ASHARE).providers == ["eastmoney", "akshare", "tencent"]
    assert get_policy(Capability.COMPANY_PROFILE, Market.HKEX).providers == ["eastmoney", "tencent", "akshare"]
    assert get_policy(Capability.FINANCIALS, Market.ASHARE).providers == ["akshare", "eastmoney", "tencent"]
    assert get_policy(Capability.DIVIDENDS, Market.ASHARE).providers == ["akshare", "eastmoney", "tencent"]


def test_registry_support_matrix():
    registry = SourceRegistry()
    assert registry.supports("tencent", Market.ASHARE, Capability.DAILY_BARS)
    assert registry.supports("akshare", Market.HKEX, Capability.DAILY_BARS)
    # 港股日线现已注册 eastmoney 作为 fallback，消除「港股单点依赖腾讯」
    assert registry.supports("eastmoney", Market.HKEX, Capability.DAILY_BARS)
    assert registry.supports("akshare", Market.ASHARE, Capability.FINANCIALS)
    assert registry.supports("eastmoney", Market.ASHARE, Capability.DIVIDENDS)
    assert registry.supports("eastmoney", Market.HKEX, Capability.COMPANY_PROFILE)


def test_manager_fallbacks_after_provider_failure():
    manager = DataSourceManager(providers={
        "tencent": StubProvider(exc=RuntimeError("boom")),
        "eastmoney": StubProvider(rows=[bar(provider="eastmoney")]),
        "akshare": StubProvider(rows=[bar(provider="akshare")]),
    })

    resp = manager.fetch(DataSourceRequest(
        capability=Capability.DAILY_BARS,
        market=Market.ASHARE,
        symbol="000001",
        target_trade_date="2026-07-10",
    ))

    assert resp.ok is True
    assert resp.used_sources == ["eastmoney"]
    assert [t.status for t in resp.trace] == ["failed", "success"]
    assert resp.data[0].provider == "eastmoney"


def test_manager_skips_unsupported_provider_market():
    manager = DataSourceManager(providers={
        "tencent": StubProvider(exc=RuntimeError("tencent down")),
        "akshare": StubProvider(rows=[bar(provider="akshare")]),
    })

    resp = manager.fetch(DataSourceRequest(
        capability=Capability.DAILY_BARS,
        market=Market.HKEX,
        symbol="00700",
        target_trade_date="2026-07-10",
    ))

    assert resp.ok is True
    assert [t.provider for t in resp.trace] == ["tencent", "eastmoney", "akshare"]
    assert resp.trace[1].status == "skipped"
    assert resp.used_sources == ["akshare"]


def test_manager_returns_partial_failure_when_all_providers_fail():
    manager = DataSourceManager(providers={
        "tencent": StubProvider(exc=RuntimeError("down1")),
        "eastmoney": StubProvider(exc=RuntimeError("down2")),
        "akshare": StubProvider(exc=RuntimeError("down3")),
    })

    resp = manager.fetch(DataSourceRequest(
        capability=Capability.INDEX_BARS,
        market=Market.ASHARE,
        symbol="000985",
        target_trade_date="2026-07-10",
    ))

    assert resp.ok is False
    assert resp.partial is True
    assert len(resp.errors) == 3
    assert all(t.status == "failed" for t in resp.trace)


def test_manager_supports_financials_fallback():
    manager = DataSourceManager(providers={
        "akshare": StubProvider(exc=RuntimeError("ak fail")),
        "eastmoney": StubProvider(rows=[{"code": "000001", "report_period": "2026-03-31"}]),
        "tencent": StubProvider(rows=[{"code": "000001", "report_period": "2026-03-31"}]),
    })

    resp = manager.fetch(DataSourceRequest(
        capability=Capability.FINANCIALS,
        market=Market.ASHARE,
        symbol="000001",
    ))

    assert resp.ok is True
    assert resp.used_sources == ["eastmoney"]
    assert resp.data[0]["code"] == "000001"


def test_manager_supports_dividends_fallback():
    manager = DataSourceManager(providers={
        "akshare": StubProvider(exc=RuntimeError("ak fail")),
        "eastmoney": StubProvider(exc=RuntimeError("em fail")),
        "tencent": StubProvider(rows=[{"code": "000001", "report_period": "2025-12-31"}]),
    })

    resp = manager.fetch(DataSourceRequest(
        capability=Capability.DIVIDENDS,
        market=Market.ASHARE,
        symbol="000001",
    ))

    assert resp.ok is True
    assert resp.used_sources == ["tencent"]
    assert resp.data[0]["report_period"] == "2025-12-31"


def test_manager_supports_company_profile():
    manager = DataSourceManager(providers={
        "eastmoney": StubProvider(rows={"symbol": "600519.SH", "exchange": "SSE"}),
        "akshare": StubProvider(rows={"symbol": "600519.SH", "exchange": "SSE"}),
        "tencent": StubProvider(rows={"symbol": "600519.SH", "exchange": "SSE"}),
    })

    resp = manager.fetch(DataSourceRequest(
        capability=Capability.COMPANY_PROFILE,
        market=Market.ASHARE,
        symbol="600519.SH",
    ))

    assert resp.ok is True
    assert resp.used_sources == ["eastmoney"]
    assert resp.data["symbol"] == "600519.SH"


def test_validate_daily_bars_requires_exact_trade_date():
    with pytest.raises(TradeDateMismatchError):
        validate_daily_bars([bar("2026-07-09")], target_trade_date="2026-07-10", require_exact_trade_date=True)


def test_manager_end_date_does_not_require_exact_trade_date():
    manager = DataSourceManager(providers={
        "tencent": StubProvider(rows=[bar("2026-07-09", provider="tencent")]),
    })

    resp = manager.fetch_daily_bars(
        symbol="000001",
        market=Market.ASHARE,
        end_date="2026-07-10",
        lookback_days=120,
    )

    assert resp.ok is True
    assert resp.data[0].trade_date == "2026-07-09"
    assert resp.source_trade_date == "2026-07-09"


def test_validate_daily_bars_rejects_invalid_price():
    bad = DailyBar(symbol="000001", market=Market.ASHARE, trade_date="2026-07-10", open=0, close=11, high=12, low=9)
    with pytest.raises(ValidationError):
        validate_daily_bars([bad])


def test_validate_daily_bars_drops_single_bad_bar():
    """单根脏 bar 应被丢弃而非整只 reject（恢复旧直连逻辑的韧性）。"""
    good = DailyBar(symbol="000001", market=Market.ASHARE, trade_date="2026-07-10", open=10, close=11, high=12, low=9)
    bad = DailyBar(symbol="000001", market=Market.ASHARE, trade_date="2026-07-09", open=0, close=11, high=12, low=9)
    cleaned = validate_daily_bars([good, bad])
    assert len(cleaned) == 1
    assert cleaned[0].trade_date == "2026-07-10"


def test_validate_daily_bars_raises_when_all_bad():
    """全部脏数据仍应硬失败，但给出被丢弃样例。"""
    bars = [
        DailyBar(symbol="000001", market=Market.ASHARE, trade_date="2026-07-10", open=0, close=0, high=0, low=0),
        DailyBar(symbol="000001", market=Market.ASHARE, trade_date="2026-07-09", open=1, close=2, high=1, low=3),
    ]
    with pytest.raises(ValidationError):
        validate_daily_bars(bars)


def test_eastmoney_hk_secid_format():
    """港股日线 secid 应使用 116 前缀 + 5 位零填充代码。"""
    from data_sources.providers.eastmoney import _secid

    assert _secid("00700", False, Market.HKEX) == "116.00700"
    assert _secid("700", False, Market.HKEX) == "116.00700"
    # A 股格式不受影响
    assert _secid("600519", False, Market.ASHARE) == "1.600519"
    assert _secid("000001", False, Market.ASHARE) == "0.000001"


def test_manager_attempts_eastmoney_for_hkex_daily():
    """港股日线主源失败时，manager 应尝试 eastmoney（已注册为 fallback）。"""
    manager = DataSourceManager(providers={
        "tencent": StubProvider(exc=RuntimeError("tencent down")),
        "eastmoney": StubProvider(rows=[bar("2026-07-10", provider="eastmoney")]),
        "akshare": StubProvider(exc=RuntimeError("akshare down")),
    })

    resp = manager.fetch(DataSourceRequest(
        capability=Capability.DAILY_BARS,
        market=Market.HKEX,
        symbol="00700",
        target_trade_date="2026-07-10",
    ))

    assert resp.ok is True
    assert resp.used_sources == ["eastmoney"]


def test_tencent_provider_backoff_on_429():
    """腾讯返回 429 时应退避重试，最终成功而不直接失败。"""
    from unittest.mock import MagicMock, patch

    from data_sources.providers.tencent import TencentProvider

    class FakeResponse:
        def __init__(self, status_code=200, json_data=None):
            self.status_code = status_code
            self._json = json_data or {}
            self.headers = {}
            self.text = ""

        def raise_for_status(self):
            if self.status_code >= 400:
                raise requests.HTTPError(f"HTTP {self.status_code}")

        def json(self):
            return self._json

    ok_payload = {
        "data": {
            "hk00700": {
                "qfqday": [["2026-07-10", "400.0", "410.0", "415.0", "398.0", "1000"]],
            }
        }
    }
    responses = [
        FakeResponse(status_code=429),
        FakeResponse(status_code=429),
        FakeResponse(status_code=200, json_data=ok_payload),
    ]

    provider = TencentProvider()
    with patch.object(requests, "get", side_effect=responses) as mock_get, \
         patch("data_sources.providers.tencent.time.sleep", return_value=None):
        bars = provider.fetch(DataSourceRequest(
            capability=Capability.DAILY_BARS,
            market=Market.HKEX,
            symbol="00700",
        ))

    assert len(bars) == 1
    assert bars[0].symbol == "00700"
    # 两次 429 重试 + 一次成功 = 3 次请求
    assert mock_get.call_count == 3
