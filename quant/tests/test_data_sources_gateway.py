import pytest

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


def test_registry_support_matrix():
    registry = SourceRegistry()
    assert registry.supports("tencent", Market.ASHARE, Capability.DAILY_BARS)
    assert registry.supports("akshare", Market.HKEX, Capability.DAILY_BARS)
    assert not registry.supports("eastmoney", Market.HKEX, Capability.DAILY_BARS)


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


def test_validate_daily_bars_requires_exact_trade_date():
    with pytest.raises(TradeDateMismatchError):
        validate_daily_bars([bar("2026-07-09")], target_trade_date="2026-07-10", require_exact_trade_date=True)


def test_validate_daily_bars_rejects_invalid_price():
    bad = DailyBar(symbol="000001", market=Market.ASHARE, trade_date="2026-07-10", open=0, close=11, high=12, low=9)
    with pytest.raises(ValidationError):
        validate_daily_bars([bad])
