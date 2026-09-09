# -*- coding: utf-8 -*-
"""C6 回放数据加载：通过 DataSourceManager 拉取 25 只行业 ETF + 基准日线。

职责边界：
- 只负责「取数与标准化为引擎输入」，不做任何策略计算。
- 降级链路由 DataSourceManager 承担（A股 baostock→tencent→eastmoney→akshare）。
- 任一资产取数失败不静默跳过：记录进 trace/errors，由调用方（Go 后端
  validation 门禁）根据 quality 报告决定是否放行 release。
"""
from __future__ import annotations

import logging
from typing import Any, Dict, List, Optional, Tuple

from data_sources import DataSourceManager, Market

from .c6_rs_rotation import C6_BENCHMARK, C6_UNIVERSE

logger = logging.getLogger(__name__)


def _normalize_date(raw: str) -> str:
    """把 provider 返回的日期统一为 YYYY-MM-DD。"""
    text = str(raw or "").strip()
    if len(text) == 8 and text.isdigit():
        return f"{text[:4]}-{text[4:6]}-{text[6:]}"
    return text


def _bars_from_response(response) -> List[Dict[str, Any]]:
    bars: List[Dict[str, Any]] = []
    for bar in response.data or []:
        item = bar.to_dict() if hasattr(bar, "to_dict") else dict(bar)
        close = item.get("close")
        if close is None:
            continue
        bars.append({"date": _normalize_date(item.get("date") or item.get("trade_date")),
                     "close": float(close)})
    bars.sort(key=lambda b: b["date"])
    # 同一日期多条记录时保留最后一条（provider 偶发重复），但不伪造新日期。
    dedup: Dict[str, Dict[str, Any]] = {}
    for bar in bars:
        dedup[bar["date"]] = bar
    return [dedup[d] for d in sorted(dedup)]


def load_c6_inputs(manager: DataSourceManager, start_date: str, end_date: str,
                   universe: Optional[List[Dict[str, str]]] = None,
                   benchmark: Optional[Dict[str, str]] = None
                   ) -> Tuple[List[Dict[str, Any]], Dict[str, Any], Dict[str, Any]]:
    """拉取 C6 全部输入。

    Args:
        manager: DataSourceManager 实例（含 health/quota 全局状态）。
        start_date/end_date: YYYYMMDD 或 YYYY-MM-DD；end_date 为空表示至今。
        universe/benchmark: 可注入替代宇宙（测试用），默认 C6 锁定宇宙。

    Returns:
        (assets, benchmark_payload, trace)。trace 含每只标的的命中源、行数与错误。
    """
    universe = universe or C6_UNIVERSE
    benchmark = benchmark or C6_BENCHMARK
    start = str(start_date or "").replace("-", "")
    end = str(end_date or "").replace("-", "")

    trace: Dict[str, Any] = {"symbols": [], "errors": []}
    assets: List[Dict[str, Any]] = []

    def fetch_bars(symbol: str, label: str) -> List[Dict[str, Any]]:
        response = manager.fetch_daily_bars(
            symbol=symbol, market=Market.ASHARE,
            start_date=start, end_date=end, adjust="qfq",
        )
        entry = {"symbol": symbol, "name": label, "ok": bool(response.ok),
                 "provider": getattr(response, "provider", ""),
                 "rows": len(response.data or []),
                 "errors": list(response.errors or [])}
        trace["symbols"].append(entry)
        if not response.ok:
            trace["errors"].append(f"{label}({symbol}): {' | '.join(entry['errors']) or '无数据'}")
            return []
        return _bars_from_response(response)

    for item in universe:
        code = str(item["code"])
        assets.append({
            "code": code,
            "name": str(item.get("name") or code),
            "list_date": str(item.get("list_date") or ""),
            "bars": fetch_bars(code, str(item.get("name") or code)),
        })

    bench_code = str(benchmark["code"])
    benchmark_payload = {
        "code": bench_code,
        "name": str(benchmark.get("name") or bench_code),
        "bars": fetch_bars(bench_code, str(benchmark.get("name") or bench_code)),
    }
    return assets, benchmark_payload, trace
