from __future__ import annotations

from datetime import datetime, timedelta
from typing import List

from ..models import Capability, DataSourceRequest, DailyBar, Market
from ..normalizers.daily_bars import normalize_akshare_frame


class AkShareProvider:
    name = "akshare"

    def fetch(self, request: DataSourceRequest) -> List[DailyBar]:
        if request.capability not in {Capability.DAILY_BARS, Capability.INDEX_BARS}:
            raise ValueError(f"AKShare 不支持能力 {request.capability}")
        import akshare as ak

        start, end = _date_range(request)
        code = str(request.symbol or "").upper().replace(".SH", "").replace(".SZ", "").replace(".HK", "")
        if request.capability == Capability.INDEX_BARS:
            if request.market != Market.ASHARE:
                raise ValueError("AKShare 指数日线第一期仅启用 A 股")
            df = ak.index_zh_a_hist(symbol=code.zfill(6), period="daily", start_date=start, end_date=end)
        elif request.market == Market.HKEX:
            df = ak.stock_hk_hist(symbol=code.zfill(5), period="daily", start_date=start, end_date=end, adjust=request.adjust)
        else:
            df = ak.stock_zh_a_hist(symbol=code.zfill(6), period="daily", start_date=start, end_date=end, adjust=request.adjust)
        return normalize_akshare_frame(df, symbol=request.symbol, market=request.market, provider=self.name)


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
