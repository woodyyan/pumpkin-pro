from __future__ import annotations

import math
from datetime import datetime, timezone
from typing import Any, Dict, Iterable, List, Optional, Tuple

from .models import CapitalMapSector, CapitalMapSnapshot, CapitalMapStock

DEFAULT_PE_BIN_SIZE = 5
DEFAULT_MAX_PE = 120
CHART_STOCK_LIMIT = 1200
SECTOR_LIMIT = 14
INFLOW_SECTOR_LIMIT = 8
REFRESH_HINT_SECONDS = 1800
SOURCE_NOTE = "当前按成交额排序抓取高流动性样本。主力净流入属于平台算法口径，不等同于交易所逐笔资金流。本页仅用于市场观察和产品验证，不构成投资建议。"


def round_value(value: Optional[float], digits: int = 2) -> Optional[float]:
    if value is None:
        return None
    if math.isnan(value) or math.isinf(value):
        return None
    return round(float(value), digits)


def calculate_poc(stocks: Iterable[CapitalMapStock], bin_size: float = DEFAULT_PE_BIN_SIZE, max_pe: float = DEFAULT_MAX_PE) -> Tuple[Optional[Dict[str, Any]], List[Dict[str, Any]]]:
    if bin_size <= 0:
        bin_size = DEFAULT_PE_BIN_SIZE
    if max_pe <= 0:
        max_pe = DEFAULT_MAX_PE

    bins: Dict[str, Dict[str, Any]] = {}
    for stock in stocks:
        if stock.pe is None or stock.pe <= 0 or stock.pe > max_pe or stock.amount <= 0:
            continue
        left = math.floor(stock.pe / bin_size) * bin_size
        right = left + bin_size
        key = f"{left:.0f}-{right:.0f}"
        bucket = bins.setdefault(key, {"key": key, "left": left, "right": right, "stocks": [], "totalAmount": 0.0, "totalPctChg": 0.0})
        bucket["stocks"].append(stock)
        bucket["totalAmount"] += stock.amount
        bucket["totalPctChg"] += stock.pct_chg or 0.0

    distribution: List[Dict[str, Any]] = []
    for bucket in bins.values():
        top_stocks = sorted(bucket["stocks"], key=lambda item: item.amount, reverse=True)[:8]
        count = len(bucket["stocks"])
        distribution.append({
            "key": bucket["key"],
            "left": bucket["left"],
            "right": bucket["right"],
            "stockCount": count,
            "totalAmount": bucket["totalAmount"],
            "totalAmountYi": round_value(bucket["totalAmount"] / 100000000, 2),
            "avgPctChg": round_value(bucket["totalPctChg"] / count, 2) if count else None,
            "topStocks": [
                {
                    "code": stock.code,
                    "symbol": stock.symbol,
                    "name": stock.name,
                    "pe": round_value(stock.pe, 2),
                    "amountYi": stock.amount_yi,
                    "pctChg": stock.pct_chg,
                }
                for stock in top_stocks
            ],
        })
    distribution.sort(key=lambda item: item["left"])
    poc = max(distribution, key=lambda item: item["totalAmount"], default=None)
    return poc, distribution


def build_market_payload(snapshot: CapitalMapSnapshot, *, cache_status: str = "fresh", last_error: str = "") -> Dict[str, Any]:
    stocks = list(snapshot.stocks or [])
    sectors = list(snapshot.sectors or [])
    total_amount = sum(stock.amount for stock in stocks)
    up_count = sum(1 for stock in stocks if (stock.pct_chg or 0) > 0)
    down_count = sum(1 for stock in stocks if (stock.pct_chg or 0) < 0)
    positive_pe_stocks = [stock for stock in stocks if stock.pe is not None and 0 < stock.pe <= DEFAULT_MAX_PE and stock.amount > 0]
    poc, distribution = calculate_poc(stocks)

    chart_stocks = sorted(positive_pe_stocks, key=lambda item: item.amount, reverse=True)[:CHART_STOCK_LIMIT]
    top_sectors = sorted(sectors, key=lambda item: item.amount, reverse=True)[:SECTOR_LIMIT]
    sector_items: List[Dict[str, Any]] = []
    for sector in top_sectors:
        ratio = round_value((sector.amount / total_amount) * 100, 2) if total_amount > 0 else None
        sector_items.append(CapitalMapSector(**{**sector.__dict__, "amount_ratio": ratio}).to_dict())

    inflow_items = [sector.to_dict() for sector in sorted(sectors, key=lambda item: item.main_net_inflow, reverse=True)[:INFLOW_SECTOR_LIMIT]]
    stock_count = snapshot.total_available if snapshot.total_available > 0 else len(stocks)
    sample_scope = snapshot.sample_scope or f"成交额前 {len(stocks)} 只股票"
    computed_at = snapshot.computed_at or datetime.now(timezone.utc).isoformat()

    payload = {
        "source": snapshot.source,
        "sourceNote": SOURCE_NOTE,
        "updatedAt": computed_at,
        "refreshHintSeconds": REFRESH_HINT_SECONDS,
        "sampleScope": sample_scope,
        "cacheStatus": cache_status,
        "market": {
            "stockCount": stock_count,
            "sampleCount": len(stocks),
            "positivePeCount": len(positive_pe_stocks),
            "chartStockCount": len(chart_stocks),
            "totalAmountYi": round_value(total_amount / 100000000, 2),
            "upCount": up_count,
            "downCount": down_count,
            "flatCount": len(stocks) - up_count - down_count,
            "upRatio": round_value((up_count / len(stocks)) * 100, 2) if stocks else None,
        },
        "stocks": [stock.to_dict() for stock in chart_stocks],
        "sectors": sector_items,
        "inflowSectors": inflow_items,
        "poc": poc,
        "pocDistribution": distribution,
    }
    if last_error:
        payload["lastError"] = last_error
    return payload


class CapitalMapService:
    def __init__(self, provider=None):
        if provider is None:
            from data_sources import DataSourceManager
            provider = DataSourceManager()
        self.provider = provider

    def get_payload(self) -> Dict[str, Any]:
        response = self.provider.fetch_capital_map()
        if not response.ok or response.data is None:
            detail = " | ".join(response.errors) if response.errors else "资金星图数据源不可用"
            raise RuntimeError(detail)
        return build_market_payload(response.data)
