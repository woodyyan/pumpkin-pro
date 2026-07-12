from __future__ import annotations

from dataclasses import dataclass, field
from datetime import datetime, timezone
from typing import Any, Dict, List, Optional


class Capability:
    DAILY_BARS = "daily_bars"
    INDEX_BARS = "index_bars"


class Market:
    ASHARE = "ASHARE"
    HKEX = "HKEX"


@dataclass(frozen=True)
class DataSourceRequest:
    capability: str
    market: str
    symbol: str
    start_date: str = ""
    end_date: str = ""
    target_trade_date: str = ""
    lookback_days: int = 120
    adjust: str = "qfq"
    require_exact_trade_date: bool = False
    allow_partial: bool = True


@dataclass(frozen=True)
class DailyBar:
    symbol: str
    market: str
    trade_date: str
    open: float
    close: float
    high: float
    low: float
    volume: float = 0.0
    amount: Optional[float] = None
    turnover_rate: Optional[float] = None
    provider: str = ""

    def to_dict(self) -> Dict[str, Any]:
        return {
            "symbol": self.symbol,
            "market": self.market,
            "trade_date": self.trade_date,
            "date": self.trade_date,  # compatibility with existing quant callers
            "open": self.open,
            "close": self.close,
            "high": self.high,
            "low": self.low,
            "volume": self.volume,
            "amount": self.amount,
            "turnover_rate": self.turnover_rate,
            "provider": self.provider,
        }


@dataclass(frozen=True)
class SourceTrace:
    provider: str
    capability: str
    market: str
    status: str
    reason: str = ""
    duration_ms: float = 0.0
    records_count: int = 0
    trade_date: str = ""
    missing_fields: List[str] = field(default_factory=list)


@dataclass(frozen=True)
class DataSourceResponse:
    ok: bool
    capability: str
    market: str
    symbol: str
    data: List[DailyBar] = field(default_factory=list)
    used_sources: List[str] = field(default_factory=list)
    trace: List[SourceTrace] = field(default_factory=list)
    errors: List[str] = field(default_factory=list)
    warnings: List[str] = field(default_factory=list)
    partial: bool = False
    source_trade_date: str = ""
    computed_at: str = field(default_factory=lambda: datetime.now(timezone.utc).isoformat())
