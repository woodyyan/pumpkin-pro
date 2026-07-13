from __future__ import annotations

import logging
import time
from datetime import datetime, timedelta
from typing import Any, List, Optional

import requests

from ..models import Capability, DataSourceRequest, DailyBar, Market
from ..normalizers.daily_bars import normalize_tencent_klines
from .company_profile_legacy import LegacyCompanyProfileProvider
from .fundamentals_legacy import LegacyFundamentalsProvider

logger = logging.getLogger(__name__)

TENCENT_KLINE_URL = "https://web.ifzq.gtimg.cn/appstock/app/fqkline/get"
HEADERS = {
    "User-Agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/131 Safari/537.36",
    "Referer": "https://stockapp.finance.qq.com/",
}

# 限流自愈：对 HTTP 403/429 退避重试，避免全量刷新时单批次请求把 IP 打进封禁。
TENCENT_MAX_RETRIES = 3
TENCENT_BACKOFF_BASE_S = 2.0


class TencentProvider:
    name = "tencent"

    def fetch(self, request: DataSourceRequest) -> List[DailyBar]:
        if request.capability == Capability.COMPANY_PROFILE:
            return LegacyCompanyProfileProvider().fetch(request.symbol, request.market)
        if request.capability in {Capability.FUNDAMENTALS, Capability.FINANCIALS, Capability.DIVIDENDS}:
            legacy = LegacyFundamentalsProvider()
            return legacy.fetch(DataSourceRequest(
                capability=request.capability,
                market=request.market,
                symbol=request.symbol,
                start_date=request.start_date,
                end_date=request.end_date,
                target_trade_date=request.target_trade_date,
                lookback_days=request.lookback_days,
                adjust=request.adjust,
                require_exact_trade_date=request.require_exact_trade_date,
                allow_partial=request.allow_partial,
                extras={**request.extras, "provider": self.name, "provider_label": self.name},
            ))
        if request.capability not in {Capability.DAILY_BARS, Capability.INDEX_BARS}:
            raise ValueError(f"Tencent 不支持能力 {request.capability}")
        symbol = _to_tencent_symbol(request.symbol, request.market, request.capability)
        start, end = _date_range(request)
        fq = "qfq" if request.adjust == "qfq" else ""
        url = f"{TENCENT_KLINE_URL}?param={symbol},day,{start},{end},500,{fq}"

        last_exc: Optional[Exception] = None
        for attempt in range(TENCENT_MAX_RETRIES):
            try:
                response = requests.get(url, headers=HEADERS, timeout=15)
                if response.status_code in (403, 429):
                    wait = self._backoff_seconds(response, attempt)
                    logger.warning(
                        "[tencent] 被限流 (HTTP %d) symbol=%s，第 %d/%d 次重试前等待 %.1fs",
                        response.status_code, request.symbol, attempt + 1, TENCENT_MAX_RETRIES, wait,
                    )
                    if attempt < TENCENT_MAX_RETRIES - 1:
                        time.sleep(wait)
                        continue
                    # 最后一次仍被限流：显式失败，让 manager 走 fallback
                    response.raise_for_status()
                response.raise_for_status()
                data: dict[str, Any] = response.json().get("data") or {}
                stock_data = data.get(symbol) or {}
                klines = stock_data.get("qfqday") or stock_data.get("day") or []
                return normalize_tencent_klines(klines, symbol=request.symbol, market=request.market, provider=self.name)
            except requests.RequestException as exc:
                last_exc = exc
                # 连接/超时类错误也退避重试一次，最后一次直接抛出走 fallback
                if attempt < TENCENT_MAX_RETRIES - 1:
                    time.sleep(self._backoff_seconds(None, attempt))
                    continue
                raise
        if last_exc is not None:
            raise last_exc
        return []

    @staticmethod
    def _backoff_seconds(response, attempt: int) -> float:
        # 优先尊重服务端 Retry-After（秒），否则指数退避 2/4/8...
        if response is not None:
            retry_after = response.headers.get("Retry-After")
            if retry_after:
                try:
                    return float(retry_after)
                except (TypeError, ValueError):
                    pass
        return TENCENT_BACKOFF_BASE_S * (2 ** attempt)


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
