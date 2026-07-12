from __future__ import annotations

from datetime import datetime
from typing import Any, Iterable, List, Optional

import pandas as pd

from ..models import DailyBar


def parse_date(value: Any) -> str:
    if value is None:
        return ""
    if hasattr(value, "strftime"):
        return value.strftime("%Y-%m-%d")
    text = str(value).strip().replace("/", "-")
    if not text:
        return ""
    if len(text) >= 10 and text[4:5] == "-":
        try:
            return datetime.fromisoformat(text[:10]).strftime("%Y-%m-%d")
        except ValueError:
            return ""
    if len(text) >= 8 and text[:8].isdigit():
        return f"{text[:4]}-{text[4:6]}-{text[6:8]}"
    return ""


def safe_float(value: Any) -> Optional[float]:
    try:
        if value is None or value == "" or str(value).strip() in {"-", "--"}:
            return None
        parsed = float(value)
        if pd.isna(parsed):
            return None
        return parsed
    except (TypeError, ValueError):
        return None


def normalize_mapping_rows(rows: Iterable[dict[str, Any]], *, symbol: str, market: str, provider: str) -> List[DailyBar]:
    result: List[DailyBar] = []
    for row in rows or []:
        trade_date = parse_date(row.get("trade_date") or row.get("date") or row.get("日期"))
        open_price = safe_float(row.get("open") or row.get("开盘")) or 0.0
        close = safe_float(row.get("close") or row.get("收盘")) or 0.0
        high = safe_float(row.get("high") or row.get("最高")) or 0.0
        low = safe_float(row.get("low") or row.get("最低")) or 0.0
        result.append(DailyBar(
            symbol=symbol,
            market=market,
            trade_date=trade_date,
            open=open_price,
            close=close,
            high=high,
            low=low,
            volume=safe_float(row.get("volume") or row.get("成交量")) or 0.0,
            amount=safe_float(row.get("amount") or row.get("成交额")),
            turnover_rate=safe_float(row.get("turnover_rate") or row.get("换手率")),
            provider=provider,
        ))
    return result


def normalize_tencent_klines(klines: Iterable[list[Any]], *, symbol: str, market: str, provider: str) -> List[DailyBar]:
    rows = []
    for item in klines or []:
        if len(item) < 6:
            continue
        rows.append({
            "date": item[0],
            "open": item[1],
            "close": item[2],
            "high": item[3],
            "low": item[4],
            "volume": item[5],
            "amount": item[6] if len(item) > 6 else None,
        })
    return normalize_mapping_rows(rows, symbol=symbol, market=market, provider=provider)


def normalize_eastmoney_klines(klines: Iterable[str], *, symbol: str, market: str, provider: str) -> List[DailyBar]:
    rows = []
    for raw in klines or []:
        parts = str(raw or "").split(",")
        if len(parts) < 7:
            continue
        rows.append({
            "date": parts[0],
            "open": parts[1],
            "close": parts[2],
            "high": parts[3],
            "low": parts[4],
            "volume": parts[5],
            "amount": parts[6],
            "turnover_rate": parts[10] if len(parts) > 10 else None,
        })
    return normalize_mapping_rows(rows, symbol=symbol, market=market, provider=provider)


def normalize_akshare_frame(df: Any, *, symbol: str, market: str, provider: str) -> List[DailyBar]:
    if df is None or getattr(df, "empty", True):
        return []
    col_map = {
        "日期": "date", "开盘": "open", "收盘": "close", "最高": "high", "最低": "low",
        "成交量": "volume", "成交额": "amount", "换手率": "turnover_rate",
    }
    renamed = df.rename(columns={key: value for key, value in col_map.items() if key in df.columns})
    return normalize_mapping_rows(renamed.to_dict("records"), symbol=symbol, market=market, provider=provider)
