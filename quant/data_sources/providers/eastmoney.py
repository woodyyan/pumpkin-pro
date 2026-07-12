from __future__ import annotations

from datetime import datetime, timedelta
from typing import List

import requests

from ..models import Capability, DataSourceRequest, DailyBar, Market
from ..normalizers.daily_bars import normalize_eastmoney_klines

EASTMONEY_KLINE_URL = "https://push2his.eastmoney.com/api/qt/stock/kline/get"


class EastMoneyProvider:
    name = "eastmoney"

    def fetch(self, request: DataSourceRequest) -> List[DailyBar]:
        if request.market != Market.ASHARE:
            raise ValueError("东方财富日线第一期仅启用 A 股")
        if request.capability not in {Capability.DAILY_BARS, Capability.INDEX_BARS}:
            raise ValueError(f"东方财富不支持能力 {request.capability}")
        start, end = _date_range(request)
        params = {
            "secid": _secid(request.symbol, request.capability == Capability.INDEX_BARS),
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


def _secid(symbol: str, is_index: bool) -> str:
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
