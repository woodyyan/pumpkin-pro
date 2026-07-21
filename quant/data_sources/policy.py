from __future__ import annotations

from dataclasses import dataclass
from typing import Dict, List, Tuple

from .models import Capability, Market


@dataclass(frozen=True)
class SourcePolicy:
    capability: str
    market: str
    providers: List[str]
    mode: str = "fallback"
    require_exact_trade_date: bool = False
    allow_partial: bool = True


# First phase: low-frequency policy changes are code-reviewed constants, not env/admin config.
POLICIES: Dict[Tuple[str, str], SourcePolicy] = {
    (Capability.COMPANY_PROFILE, Market.ASHARE): SourcePolicy(
        capability=Capability.COMPANY_PROFILE,
        market=Market.ASHARE,
        providers=["eastmoney", "akshare", "tencent"],
    ),
    (Capability.COMPANY_PROFILE, Market.HKEX): SourcePolicy(
        capability=Capability.COMPANY_PROFILE,
        market=Market.HKEX,
        providers=["eastmoney", "tencent", "akshare"],
    ),
    (Capability.FUNDAMENTALS, Market.ASHARE): SourcePolicy(
        capability=Capability.FUNDAMENTALS,
        market=Market.ASHARE,
        providers=["akshare", "eastmoney", "tencent"],
    ),
    (Capability.FUNDAMENTALS, Market.HKEX): SourcePolicy(
        capability=Capability.FUNDAMENTALS,
        market=Market.HKEX,
        providers=["eastmoney", "tencent", "akshare"],
    ),
    (Capability.FINANCIALS, Market.ASHARE): SourcePolicy(
        capability=Capability.FINANCIALS,
        market=Market.ASHARE,
        providers=["akshare", "eastmoney", "tencent"],
    ),
    (Capability.DIVIDENDS, Market.ASHARE): SourcePolicy(
        capability=Capability.DIVIDENDS,
        market=Market.ASHARE,
        providers=["baostock", "akshare", "eastmoney", "tencent"],
    ),
    (Capability.DAILY_BARS, Market.ASHARE): SourcePolicy(
        capability=Capability.DAILY_BARS,
        market=Market.ASHARE,
        providers=["baostock", "tencent", "eastmoney", "akshare"],
        require_exact_trade_date=True,
    ),
    (Capability.DAILY_BARS, Market.HKEX): SourcePolicy(
        capability=Capability.DAILY_BARS,
        market=Market.HKEX,
        providers=["tencent", "eastmoney", "akshare"],
        require_exact_trade_date=True,
    ),
    (Capability.INDEX_BARS, Market.ASHARE): SourcePolicy(
        capability=Capability.INDEX_BARS,
        market=Market.ASHARE,
        providers=["tencent", "eastmoney", "akshare"],
        require_exact_trade_date=True,
    ),
    (Capability.INDEX_BARS, Market.HKEX): SourcePolicy(
        capability=Capability.INDEX_BARS,
        market=Market.HKEX,
        providers=["tencent", "eastmoney", "akshare"],
        require_exact_trade_date=True,
    ),
    (Capability.CAPITAL_MAP, Market.ASHARE): SourcePolicy(
        capability=Capability.CAPITAL_MAP,
        market=Market.ASHARE,
        providers=["eastmoney"],
    ),
}


def get_policy(capability: str, market: str) -> SourcePolicy:
    key = (str(capability or ""), str(market or "").upper())
    if key not in POLICIES:
        raise KeyError(f"unsupported data source policy: capability={capability} market={market}")
    return POLICIES[key]
