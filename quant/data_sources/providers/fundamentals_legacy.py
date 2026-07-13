from __future__ import annotations

from typing import Any, Dict, List

from ..models import Capability, DataSourceRequest, Market


class LegacyFundamentalsProvider:
    """Adapter over existing `data.fundamentals` logic for Phase0 migration.

    Ponytail path: reuse battle-tested legacy parsing/fallback code first, and let
    Gateway only own orchestration/trace. Replace with native provider adapters
    later when capability-specific sources are fully split out.
    """

    name = "legacy_fundamentals"

    def fetch(self, request: DataSourceRequest):
        if request.capability == Capability.FUNDAMENTALS:
            return self._fetch_fundamentals(request)
        if request.capability == Capability.FINANCIALS:
            return self._fetch_financials(request)
        if request.capability == Capability.DIVIDENDS:
            return self._fetch_dividends(request)
        raise ValueError(f"legacy fundamentals adapter 不支持能力 {request.capability}")

    @staticmethod
    def _fetch_fundamentals(request: DataSourceRequest) -> Dict[str, Any]:
        from data.fundamentals import get_symbol_fundamentals

        return get_symbol_fundamentals(request.symbol)

    @staticmethod
    def _fetch_financials(request: DataSourceRequest) -> List[Dict[str, Any]]:
        from data.fundamentals import get_financial_metrics

        code = _normalize_code(request.symbol)
        metrics = get_financial_metrics(code)
        if not metrics:
            raise RuntimeError("基础面财务数据为空")
        report_period = metrics.get("fy_report_date") or metrics.get("ttm_report_date") or ""
        return [{
            "code": code,
            "report_period": report_period,
            "report_date": metrics.get("ttm_report_date") or metrics.get("fy_report_date") or "",
            "revenue": metrics.get("revenue_fy"),
            "revenue_yoy": None,
            "net_profit": metrics.get("net_profit_fy"),
            "net_profit_yoy": metrics.get("profit_growth_rate"),
            "total_assets": None,
            "total_equity": None,
            "operating_cash_flow": None,
            "capex": None,
            "source": f"{request.extras.get('provider_label', request.extras.get('provider', 'legacy'))}:fundamentals",
        }]

    @staticmethod
    def _fetch_dividends(request: DataSourceRequest) -> List[Dict[str, Any]]:
        from data.fundamentals import get_dividend_metrics

        code = _normalize_code(request.symbol)
        metrics = get_dividend_metrics(code)
        if not metrics:
            raise RuntimeError("基础面分红数据为空")
        report_period = metrics.get("report_date") or "unknown"
        return [{
            "code": code,
            "report_period": report_period,
            "ex_dividend_date": "unknown",
            "cash_dividend_per_share": None,
            "total_cash_dividend": None,
            "dividend_yield": metrics.get("dividend_yield"),
            "dividend_yield_source": "items.dividend_yield",
            "raw_plan": "",
            "source": f"{request.extras.get('provider_label', request.extras.get('provider', 'legacy'))}:fundamentals",
        }]


def _normalize_code(symbol: str) -> str:
    text = str(symbol or "").strip().upper()
    if text.endswith(".HK"):
        return text[:-3].zfill(5)
    if text.endswith((".SH", ".SZ")):
        return text[:-3].zfill(6)
    if text.isdigit():
        if len(text) == 5:
            return text.zfill(5)
        return text.zfill(6)
    digits = "".join(ch for ch in text if ch.isdigit())
    if len(digits) == 5:
        return digits.zfill(5)
    return digits.zfill(6)


def infer_market_from_symbol(symbol: str) -> str:
    text = str(symbol or "").strip().upper()
    if text.endswith(".HK") or len("".join(ch for ch in text if ch.isdigit())) == 5:
        return Market.HKEX
    return Market.ASHARE
