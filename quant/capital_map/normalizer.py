from __future__ import annotations

import json
from typing import Any, Optional

import math

from .models import CapitalMapSector, CapitalMapStock


def round_value(value: Optional[float], digits: int = 2) -> Optional[float]:
    if value is None or math.isnan(value) or math.isinf(value):
        return None
    return round(float(value), digits)


def field_string(item: dict[str, Any], key: str) -> str:
    value = item.get(key)
    if value is None or value == "-":
        return ""
    if isinstance(value, float):
        return f"{value:.0f}"
    return str(value).strip()


def field_float(item: dict[str, Any], key: str) -> Optional[float]:
    value = item.get(key)
    if value is None:
        return None
    if isinstance(value, (int, float)):
        return float(value)
    text = str(value).strip()
    if not text or text == "-":
        return None
    try:
        return float(json.loads(text))
    except Exception:
        try:
            return float(text)
        except ValueError:
            return None


def market_prefix(code: str, market: Optional[float]) -> str:
    if code.startswith("6") or market == 1:
        return "SH"
    if code.startswith(("8", "9")):
        return "BJ"
    return "SZ"


def normalize_capital_map_stock(item: dict[str, Any]) -> CapitalMapStock:
    code = field_string(item, "f12")
    dynamic_pe = field_float(item, "f9")
    pe_ttm = field_float(item, "f115")
    selected_pe = dynamic_pe
    pe_source = "动态PE"
    if pe_ttm is not None and pe_ttm > 0:
        selected_pe = pe_ttm
        pe_source = "PE TTM"
    amount = field_float(item, "f6") or 0.0
    market = market_prefix(code, field_float(item, "f13"))
    main_net_inflow = field_float(item, "f62")
    total_market_cap = field_float(item, "f20")
    float_market_cap = field_float(item, "f21")
    return CapitalMapStock(
        code=code,
        symbol=f"{market}{code}",
        name=field_string(item, "f14"),
        market=market,
        price=field_float(item, "f2"),
        pct_chg=field_float(item, "f3"),
        amount=amount,
        amount_yi=round_value(amount / 100000000, 2),
        volume_hands=field_float(item, "f5"),
        turnover_rate=field_float(item, "f8"),
        pe=selected_pe,
        pe_ttm=pe_ttm,
        dynamic_pe=dynamic_pe,
        pe_source=pe_source,
        pb=field_float(item, "f23"),
        total_market_cap=total_market_cap,
        float_market_cap=float_market_cap,
        total_market_cap_yi=round_value((total_market_cap or 0.0) / 100000000, 2),
        float_market_cap_yi=round_value((float_market_cap or 0.0) / 100000000, 2),
        main_net_inflow=main_net_inflow,
        main_net_inflow_yi=round_value((main_net_inflow or 0.0) / 100000000, 2),
        change_60d=field_float(item, "f24"),
        change_ytd=field_float(item, "f25"),
    )


def normalize_capital_map_sector(item: dict[str, Any]) -> CapitalMapSector:
    amount = field_float(item, "f6") or 0.0
    main_net_inflow = field_float(item, "f62") or 0.0
    return CapitalMapSector(
        code=field_string(item, "f12"),
        name=field_string(item, "f14"),
        pct_chg=field_float(item, "f3"),
        amount=amount,
        amount_yi=round_value(amount / 100000000, 2),
        main_net_inflow=main_net_inflow,
        main_net_inflow_yi=round_value(main_net_inflow / 100000000, 2),
        net_inflow_intensity=round_value((main_net_inflow / amount) * 100, 2) if amount > 0 else None,
        leader_name=field_string(item, "f128"),
        leader_code=field_string(item, "f140"),
    )
