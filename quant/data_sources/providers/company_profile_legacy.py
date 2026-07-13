from __future__ import annotations

from typing import Any, Dict

from data.company_profile import fetch_a_share_company_profile, fetch_hk_company_profile, normalize_symbol
from ..models import Market


class LegacyCompanyProfileProvider:
    name = "legacy_company_profile"

    def fetch(self, symbol: str, market: str) -> Dict[str, Any]:
        normalized, exchange, _ = normalize_symbol(symbol)
        if market == Market.HKEX or exchange == "HKEX":
            return fetch_hk_company_profile(normalized)
        return fetch_a_share_company_profile(normalized)
