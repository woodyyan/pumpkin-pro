from __future__ import annotations

from datetime import datetime, timedelta
from typing import Any, List

import requests

from ..models import Capability, DataSourceRequest, DailyBar, Market
from ..normalizers.daily_bars import normalize_tencent_klines

TENCENT_KLINE_URL = "https://web.ifzq.gtimg.cn/appstock/app/fqkline/get"
HEADERS = {
    "User-Agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/131 Safari/537.36",
    "Referer": "https://stockapp.finance.qq.com/",
}


class TencentProvider:
    name = "tencent"

    def fetch(self, request: DataSourceRequest) -> List[DailyBar]:
        if request.capability not in {Capability.DAILY_BARS, Capability.INDEX_BARS}:
            raise ValueError(f"Tencent 不支持能力 {request.capability}")
        symbol = _to_tencent_symbol(request.symbol, request.market, request.capability)
        start, end = _date_range(request)
        fq = "qfq" if request.adjust == "qfq" else ""
        url = f"{TENCENT_KLINE_URL}?param={symbol},day,{start},{end},500,{fq}"
        response = requests.get(url, headers=HEADERS, timeout=15)
        response.raise_for_status()
        data: dict[str, Any] = response.json().get("data") or {}
        stock_data = data.get(symbol) or {}
        klines = stock_data.get("qfqday") or stock_data.get("day") or []
        return normalize_tencent_klines(klines, symbol=request.symbol, market=request.market, provider=self.name)


def _to_tencent_symbol(symbol: str, market: str, capability: str) -> str:
    raw = str(symbol or "").upper().replace(".SH", "").replace(".SZ", "").replace(".HK", "")
    if market == Market.HKEX:
        if capability == Capability.INDEX_BARS and raw in {"HSI", "HANGSENG"}:
            return "hkHSI"
        return f"hk{raw.zfill(5)}"
    code = raw.zfill(6)
    if capability == Capability.INDEX_BARS:
        return f"sh{code}" if code.startswith("0") else f"sz{code}"
    return f"sh{code}" if code.startswith(("6", "9")) else f"sz{code}"


def _date_range(request: DataSourceRequest) -> tuple[str, str]:
    end = _parse_date(request.end_date or request.target_trade_date) or datetime.today()
    start = _parse_date(request.start_date) or (end - timedelta(days=request.lookback_days + 30))
    return start.strftime("%Y-%m-%d"), end.strftime("%Y-%m-%d")


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
