from __future__ import annotations

from dataclasses import dataclass, field
from datetime import datetime, timezone
from typing import Any, Dict, List, Optional


@dataclass(frozen=True)
class CapitalMapStock:
    code: str
    symbol: str
    name: str
    market: str
    price: Optional[float] = None
    pct_chg: Optional[float] = None
    amount: float = 0.0
    amount_yi: Optional[float] = None
    volume_hands: Optional[float] = None
    turnover_rate: Optional[float] = None
    pe: Optional[float] = None
    pe_ttm: Optional[float] = None
    dynamic_pe: Optional[float] = None
    pe_source: str = ""
    pb: Optional[float] = None
    total_market_cap: Optional[float] = None
    float_market_cap: Optional[float] = None
    total_market_cap_yi: Optional[float] = None
    float_market_cap_yi: Optional[float] = None
    main_net_inflow: Optional[float] = None
    main_net_inflow_yi: Optional[float] = None
    change_60d: Optional[float] = None
    change_ytd: Optional[float] = None

    def to_dict(self) -> Dict[str, Any]:
        data = {
            "code": self.code,
            "symbol": self.symbol,
            "name": self.name,
            "market": self.market,
            "price": self.price,
            "pctChg": self.pct_chg,
            "amountYi": self.amount_yi,
            "volumeHands": self.volume_hands,
            "turnoverRate": self.turnover_rate,
            "pe": self.pe,
            "peTtm": self.pe_ttm,
            "dynamicPe": self.dynamic_pe,
            "peSource": self.pe_source,
            "pb": self.pb,
            "totalMarketCapYi": self.total_market_cap_yi,
            "floatMarketCapYi": self.float_market_cap_yi,
            "mainNetInflowYi": self.main_net_inflow_yi,
            "change60d": self.change_60d,
            "changeYtd": self.change_ytd,
        }
        return {k: v for k, v in data.items() if v is not None}


@dataclass(frozen=True)
class CapitalMapSector:
    code: str
    name: str
    pct_chg: Optional[float] = None
    amount: float = 0.0
    amount_yi: Optional[float] = None
    amount_ratio: Optional[float] = None
    main_net_inflow: float = 0.0
    main_net_inflow_yi: Optional[float] = None
    net_inflow_intensity: Optional[float] = None
    leader_name: str = ""
    leader_code: str = ""

    def to_dict(self) -> Dict[str, Any]:
        data = {
            "code": self.code,
            "name": self.name,
            "pctChg": self.pct_chg,
            "amountYi": self.amount_yi,
            "amountRatio": self.amount_ratio,
            "mainNetInflowYi": self.main_net_inflow_yi,
            "netInflowIntensity": self.net_inflow_intensity,
            "leaderName": self.leader_name,
            "leaderCode": self.leader_code,
        }
        return {k: v for k, v in data.items() if v not in (None, "")}


@dataclass(frozen=True)
class CapitalMapSnapshot:
    stocks: List[CapitalMapStock] = field(default_factory=list)
    sectors: List[CapitalMapSector] = field(default_factory=list)
    total_available: int = 0
    sample_scope: str = ""
    source: str = "东方财富公开行情接口"
    computed_at: str = field(default_factory=lambda: datetime.now(timezone.utc).isoformat())
