from __future__ import annotations

from datetime import datetime, timedelta, timezone
from typing import Any, List

import requests

from capital_map.models import CapitalMapSnapshot
from capital_map.normalizer import normalize_capital_map_sector, normalize_capital_map_stock
from ..models import Capability, DataSourceRequest, DailyBar, Market
from ..normalizers.daily_bars import normalize_eastmoney_klines

EASTMONEY_KLINE_URL = "https://push2his.eastmoney.com/api/qt/stock/kline/get"
EASTMONEY_CLIST_URL = "https://82.push2.eastmoney.com/api/qt/clist/get"
EASTMONEY_SECTOR_URL = "https://push2.eastmoney.com/api/qt/clist/get"
EASTMONEY_TOKEN = "bd1d9ddb04089700cf9c27f6f7426281"
CAPITAL_MAP_PAGE_SIZE = 100
CAPITAL_MAP_PAGE_COUNT = 16
CAPITAL_MAP_STOCK_FIELDS = "f2,f3,f4,f5,f6,f7,f8,f9,f10,f12,f13,f14,f15,f16,f17,f18,f20,f21,f23,f24,f25,f62,f115"
CAPITAL_MAP_SECTOR_FIELDS = "f3,f6,f12,f14,f62,f128,f140,f141"


class EastMoneyProvider:
    name = "eastmoney"

    def fetch(self, request: DataSourceRequest):
        if request.capability == Capability.CAPITAL_MAP:
            return self.fetch_capital_map(request)
        if request.capability not in {Capability.DAILY_BARS, Capability.INDEX_BARS}:
            raise ValueError(f"东方财富不支持能力 {request.capability}")
        # 港股仅开放日线（指数待验证）；A 股日线 + 指数均支持。
        if request.market == Market.HKEX and request.capability != Capability.DAILY_BARS:
            raise ValueError("东方财富港股第一期仅启用日线")
        if request.market not in {Market.ASHARE, Market.HKEX}:
            raise ValueError("东方财富日线第一期仅启用 A 股 / 港股")
        start, end = _date_range(request)
        params = {
            "secid": _secid(request.symbol, request.capability == Capability.INDEX_BARS, request.market),
            "klt": 101,
            "fqt": "1" if request.adjust == "qfq" else ("2" if request.adjust == "hfq" else "0"),
            "beg": start,
            "end": end,
            "fields1": "f1,f2,f3,f4,f5,f6",
            "fields2": "f51,f52,f53,f54,f55,f56,f57,f58,f59,f60,f61",
        }
        response = requests.get(EASTMONEY_KLINE_URL, params=params, timeout=15)
        response.raise_for_status()
        klines = ((response.json().get("data") or {}).get("klines") or [])
        return normalize_eastmoney_klines(klines, symbol=request.symbol, market=request.market, provider=self.name)

    def fetch_capital_map(self, request: DataSourceRequest) -> CapitalMapSnapshot:
        if request.market != Market.ASHARE:
            raise ValueError("资金星图第一期仅支持 A 股")

        rows: list[dict[str, Any]] = []
        total_available = 0
        failed_pages = 0
        for page in range(1, CAPITAL_MAP_PAGE_COUNT + 1):
            try:
                response = requests.get(EASTMONEY_CLIST_URL, params=_capital_map_stock_params(page), timeout=15)
                response.raise_for_status()
                data = response.json().get("data") or {}
                if total_available == 0:
                    total_available = int(data.get("total") or 0)
                rows.extend(data.get("diff") or [])
            except Exception:
                failed_pages += 1
        if not rows:
            raise ValueError(f"东方财富资金星图股票页全部失败: {failed_pages}/{CAPITAL_MAP_PAGE_COUNT}")

        sector_response = requests.get(EASTMONEY_SECTOR_URL, params=_capital_map_sector_params(), timeout=15)
        sector_response.raise_for_status()
        sector_rows = ((sector_response.json().get("data") or {}).get("diff") or [])

        stocks = [stock for stock in (normalize_capital_map_stock(row) for row in rows) if stock.code and stock.name and stock.amount > 0]
        sectors = [sector for sector in (normalize_capital_map_sector(row) for row in sector_rows) if sector.code and sector.name and sector.amount > 0]
        scope = f"成交额前 {len(stocks)} 只股票"
        if failed_pages:
            scope += f"（{failed_pages} 页获取失败）"
        return CapitalMapSnapshot(
            stocks=stocks,
            sectors=sectors,
            total_available=total_available,
            sample_scope=scope,
            computed_at=datetime.now(timezone.utc).isoformat(),
        )


def _secid(symbol: str, is_index: bool, market: str = Market.ASHARE) -> str:
    if market == Market.HKEX:
        # 港股 secid 用 116 前缀 + 5 位零填充代码（如腾讯 00700 -> 116.00700）
        code = str(symbol or "").upper().replace(".HK", "").zfill(5)
        return f"116.{code}"
    code = str(symbol or "").upper().replace(".SH", "").replace(".SZ", "").zfill(6)
    if is_index:
        return f"1.{code}" if code.startswith("0") else f"0.{code}"
    return f"1.{code}" if code.startswith(("6", "9")) else f"0.{code}"


def _date_range(request: DataSourceRequest) -> tuple[str, str]:
    end = _parse_date(request.end_date or request.target_trade_date) or datetime.today()
    start = _parse_date(request.start_date) or (end - timedelta(days=request.lookback_days + 30))
    return start.strftime("%Y%m%d"), end.strftime("%Y%m%d")


def _parse_date(value: str) -> datetime | None:
    text = str(value or "").strip()
    if not text:
        return None
    for fmt in ("%Y-%m-%d", "%Y%m%d"):
        try:
            return datetime.strptime(text[:10] if fmt == "%Y-%m-%d" else text[:8], fmt)
        except ValueError:
            continue
    return None


def _capital_map_stock_params(page: int) -> dict[str, str]:
    return {
        "pn": str(page),
        "pz": str(CAPITAL_MAP_PAGE_SIZE),
        "po": "1",
        "np": "1",
        "ut": EASTMONEY_TOKEN,
        "fltt": "2",
        "invt": "2",
        "fid": "f6",
        "fs": "m:0+t:6,m:0+t:80,m:1+t:2,m:1+t:23,m:0+t:81+s:2048",
        "fields": CAPITAL_MAP_STOCK_FIELDS,
    }


def _capital_map_sector_params() -> dict[str, str]:
    return {
        "pn": "1",
        "pz": "80",
        "po": "1",
        "np": "1",
        "ut": EASTMONEY_TOKEN,
        "fltt": "2",
        "invt": "2",
        "fid": "f62",
        "fs": "m:90+t:2+f:!50",
        "fields": CAPITAL_MAP_SECTOR_FIELDS,
    }
