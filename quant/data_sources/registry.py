from __future__ import annotations

from dataclasses import dataclass
from typing import Dict, Iterable, Set, Tuple

from .models import Capability, Market


@dataclass(frozen=True)
class ProviderCapability:
    provider: str
    market: str
    capability: str


PROVIDER_CAPABILITIES = (
    ProviderCapability("eastmoney", Market.ASHARE, Capability.COMPANY_PROFILE),
    ProviderCapability("eastmoney", Market.HKEX, Capability.COMPANY_PROFILE),
    ProviderCapability("akshare", Market.ASHARE, Capability.COMPANY_PROFILE),
    ProviderCapability("tencent", Market.HKEX, Capability.COMPANY_PROFILE),
    ProviderCapability("eastmoney", Market.ASHARE, Capability.FUNDAMENTALS),
    ProviderCapability("eastmoney", Market.HKEX, Capability.FUNDAMENTALS),
    ProviderCapability("tencent", Market.ASHARE, Capability.FUNDAMENTALS),
    ProviderCapability("tencent", Market.HKEX, Capability.FUNDAMENTALS),
    ProviderCapability("akshare", Market.ASHARE, Capability.FUNDAMENTALS),
    ProviderCapability("eastmoney", Market.ASHARE, Capability.FINANCIALS),
    ProviderCapability("tencent", Market.ASHARE, Capability.FINANCIALS),
    ProviderCapability("akshare", Market.ASHARE, Capability.FINANCIALS),
    ProviderCapability("eastmoney", Market.ASHARE, Capability.DIVIDENDS),
    ProviderCapability("tencent", Market.ASHARE, Capability.DIVIDENDS),
    ProviderCapability("akshare", Market.ASHARE, Capability.DIVIDENDS),
    ProviderCapability("tencent", Market.ASHARE, Capability.DAILY_BARS),
    ProviderCapability("tencent", Market.HKEX, Capability.DAILY_BARS),
    ProviderCapability("tencent", Market.ASHARE, Capability.INDEX_BARS),
    ProviderCapability("tencent", Market.HKEX, Capability.INDEX_BARS),
    ProviderCapability("eastmoney", Market.ASHARE, Capability.DAILY_BARS),
    ProviderCapability("eastmoney", Market.HKEX, Capability.DAILY_BARS),
    ProviderCapability("eastmoney", Market.ASHARE, Capability.INDEX_BARS),
    ProviderCapability("akshare", Market.ASHARE, Capability.DAILY_BARS),
    ProviderCapability("akshare", Market.HKEX, Capability.DAILY_BARS),
    ProviderCapability("akshare", Market.ASHARE, Capability.INDEX_BARS),
    ProviderCapability("eastmoney", Market.ASHARE, Capability.CAPITAL_MAP),
)


class SourceRegistry:
    def __init__(self, capabilities: Iterable[ProviderCapability] = PROVIDER_CAPABILITIES):
        self._capabilities: Set[Tuple[str, str, str]] = {
            (item.provider, item.market, item.capability) for item in capabilities
        }

    def supports(self, provider: str, market: str, capability: str) -> bool:
        return (provider, str(market or "").upper(), capability) in self._capabilities

    def as_matrix(self) -> Dict[str, Dict[str, list[str]]]:
        matrix: Dict[str, Dict[str, list[str]]] = {}
        for provider, market, capability in sorted(self._capabilities):
            matrix.setdefault(provider, {}).setdefault(market, []).append(capability)
        return matrix
