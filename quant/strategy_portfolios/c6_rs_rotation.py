# -*- coding: utf-8 -*-
"""C6 行业 ETF RS 轮动 —— 平台独立回放引擎（修正版）。

来源：传递包 RS行业轮动/rs_engine.py（backtest_full）。本模块是正式站的唯一
权威实现，源端 rs_engine.js 双引擎与「上市前填充」语义全部废弃。

相对源引擎的刻意修正（差异须进入对账报告，不做强行追平）：

1. 禁止同日收益（未来函数）：
   源引擎在 T 日收盘先按新权重成交、再让新权重吃 T 日全天收益。
   本引擎改为「T 日收盘出信号 -> T+1 日收盘成交 -> T+2 日起吃收益」，
   即每日先用旧权重结算当日收益，再在收盘执行 pending 调仓。

2. 禁止上市前未来价格回填：
   源引擎 union_align 用上市首日收盘价向前填充上市前日期。本引擎按
   list_date 硬性掩蔽，上市前该资产信号视为无效、权重恒为 0。

3. 禁止缺失写零/静默伪造：
   缺价格时不补 0、不前向填充。持仓资产缺价当日收益按 0 处理（经济上
   等价于停牌），但必须计入 missing 并反映到质量报告；无价格无法成交的
   调仓记为 blocked 警告。

输入一律为「真实存在的日线」，引擎不做任何联网与文件 IO，可确定性回放。
"""
from __future__ import annotations

import hashlib
import json
import math
from typing import Any, Dict, List, Optional

ENGINE_KEY = "c6_rs_rotation"
ENGINE_VERSION = "2.0.0"

TRADING_DAYS_PER_YEAR = 252

# C6 锁定参数（路径D 最优；调整需先在网格研究中验证并升版本号）
DEFAULT_PARAMS: Dict[str, Any] = {
    "top_n": 3,                # N：持仓只数
    "rebalance_interval": 5,   # R：调仓间隔（交易日，周频）
    "lookback": 20,            # L：RS 相对强度回望窗口
    "use_timing": True,        # 沪深300 择时熔断
    "timing_ma": 120,          # 择时均线
    "use_stop": True,          # 个股跟踪止损
    "stop_pct": 0.10,          # 止损回撤比例
    "cost": 0.001,             # 单边换手率成本
    "risk_free": 0.015,        # 现金年化收益（空仓/现金部分）
}

# C6 宇宙：25 只行业 ETF + 基准 510300（沪深300ETF）。
# list_date 刻意留空：上市前掩蔽以「数据内首个真实日线」推导，
# 避免硬编码的上市日期与上游真实数据冲突导致误掩蔽。
# 如上游 provider 返回上市前合成价格，可在 params 之外通过
# assets[].list_date 显式传入权威上市日期进行硬性掩蔽。
C6_UNIVERSE: List[Dict[str, str]] = [
    {"code": "512660", "name": "军工ETF国泰", "list_date": ""},
    {"code": "512010", "name": "医药ETF易方达", "list_date": ""},
    {"code": "512880", "name": "证券ETF国泰", "list_date": ""},
    {"code": "512800", "name": "银行ETF华宝", "list_date": ""},
    {"code": "159995", "name": "芯片ETF华夏", "list_date": ""},
    {"code": "159870", "name": "化工ETF鹏华", "list_date": ""},
    {"code": "515210", "name": "钢铁ETF国泰", "list_date": ""},
    {"code": "512400", "name": "有色金属ETF南方", "list_date": ""},
    {"code": "516110", "name": "汽车ETF国泰", "list_date": ""},
    {"code": "561120", "name": "家电ETF富国", "list_date": ""},
    {"code": "515170", "name": "食品饮料ETF华夏", "list_date": ""},
    {"code": "159745", "name": "建材ETF国泰", "list_date": ""},
    {"code": "516710", "name": "新材料ETF华夏", "list_date": ""},
    {"code": "159611", "name": "电力ETF广发", "list_date": ""},
    {"code": "516950", "name": "基建ETF银华", "list_date": ""},
    {"code": "512200", "name": "房地产ETF南方", "list_date": ""},
    {"code": "515030", "name": "新能源车ETF华夏", "list_date": ""},
    {"code": "512720", "name": "计算机ETF国泰", "list_date": ""},
    {"code": "512980", "name": "传媒ETF广发", "list_date": ""},
    {"code": "515880", "name": "通信ETF国泰", "list_date": ""},
    {"code": "515220", "name": "煤炭ETF国泰", "list_date": ""},
    {"code": "561360", "name": "石油ETF国泰", "list_date": ""},
    {"code": "159861", "name": "碳中和50ETF国泰", "list_date": ""},
    {"code": "516960", "name": "机械ETF国泰", "list_date": ""},
    {"code": "159981", "name": "能源化工ETF建信", "list_date": ""},
]

C6_BENCHMARK = {"code": "510300", "name": "沪深300ETF", "list_date": ""}

_EPS = 1e-9
_INVALID_SIGNAL = -1e9


def _sma(arr: List[Optional[float]], t: int, window: int) -> Optional[float]:
    """t 时刻的 window 日均线（含 t，0-based）。任一数据缺失返回 None。"""
    if t < window - 1:
        return None
    window_values = arr[t - window + 1: t + 1]
    if any(v is None for v in window_values):
        return None
    return sum(v for v in window_values if v is not None) / window


def _round(value: Optional[float], digits: int = 6) -> Optional[float]:
    if value is None:
        return None
    return round(float(value), digits)


def compute_metrics(equity: List[float], bench: List[float],
                    axis_dates: Optional[List[str]] = None,
                    risk_free: float = DEFAULT_PARAMS["risk_free"]) -> Dict[str, Any]:
    """组合看板统一口径指标（与源 build_real.py metrics() 同名同义）。

    equity/bench 均为归一化净值序列（首元素为基线）。样本不足 60 日时，
    年化类指标返回 None，由上层决定是否展示。
    """
    n = len(equity)
    zero = dict(cum=0.0, ann=None, r20=0.0, sharpe=None, excess=0.0, mdd=0.0,
                vol=None, mth=0.0, ytd=0.0, beta=None, var95=None,
                calmar=None, sortino=None)
    if n < 2 or not equity[0]:
        return zero
    cum = equity[-1] / equity[0] - 1
    ann = (equity[-1] / equity[0]) ** (TRADING_DAYS_PER_YEAR / n) - 1 if equity[0] > 0 else 0.0
    daily_rets = [equity[i] / equity[i - 1] - 1 for i in range(1, n) if equity[i - 1] > 0]
    bench_rets = ([bench[i] / bench[i - 1] - 1 for i in range(1, n) if bench[i - 1] > 0]
                  if bench and len(bench) == n else [])
    if not daily_rets:
        return zero
    mean = sum(daily_rets) / len(daily_rets)
    var = sum((x - mean) ** 2 for x in daily_rets) / len(daily_rets)
    std = math.sqrt(var)
    vol = std * math.sqrt(TRADING_DAYS_PER_YEAR)
    sharpe = (mean / std * math.sqrt(TRADING_DAYS_PER_YEAR)) if std > 0 else None
    peak = equity[0]
    mdd = 0.0
    for value in equity:
        peak = max(peak, value)
        if peak > 0:
            mdd = min(mdd, value / peak - 1)
    beta = None
    if bench_rets and len(bench_rets) == len(daily_rets):
        bench_mean = sum(bench_rets) / len(bench_rets)
        bench_var = sum((x - bench_mean) ** 2 for x in bench_rets) / len(bench_rets)
        cov = sum((daily_rets[i] - mean) * (bench_rets[i] - bench_mean)
                  for i in range(len(daily_rets))) / len(daily_rets)
        beta = (cov / bench_var) if bench_var > 0 else None
    sorted_rets = sorted(daily_rets)
    var95 = -sorted_rets[max(0, int(len(sorted_rets) * 0.05) - 1)] if sorted_rets else None
    calmar = (ann / abs(mdd)) if mdd != 0 else None
    downside = [x for x in daily_rets if x < 0]
    downside_std = math.sqrt(sum(x * x for x in downside) / len(daily_rets)) if downside else 0.0
    sortino = (mean / downside_std * math.sqrt(TRADING_DAYS_PER_YEAR)) if downside_std > 0 else None

    mth = 0.0
    ytd = 0.0
    if axis_dates and len(axis_dates) == n:
        current_month = axis_dates[-1][:7]
        month_start = next((i for i, d in enumerate(axis_dates) if d[:7] == current_month), 0)
        if month_start < n - 1 and equity[month_start] > 0:
            mth = equity[-1] / equity[month_start] - 1
        year_start = axis_dates[-1][:4] + "-01-01"
        year_idx = next((i for i, d in enumerate(axis_dates) if d >= year_start), 0)
        if equity[year_idx] > 0:
            ytd = equity[-1] / equity[year_idx] - 1

    excess = cum - (bench[-1] / bench[0] - 1) if (bench and bench[0]) else cum
    result = dict(cum=cum, ann=ann, r20=daily_rets[-1], sharpe=sharpe, excess=excess,
                  mdd=mdd, vol=vol, mth=mth, ytd=ytd, beta=beta, var95=var95,
                  calmar=calmar, sortino=sortino)
    if n < 60:
        for key in ("ann", "vol", "sharpe", "sortino", "calmar", "var95", "beta"):
            result[key] = None
    return result


def run_c6_replay(assets: List[Dict[str, Any]], benchmark: Dict[str, Any],
                  params: Optional[Dict[str, Any]] = None) -> Dict[str, Any]:
    """C6 平台独立回放。

    Args:
        assets: [{code, name, list_date, bars: [{date, close}]}]，bars 只含真实日线。
        benchmark: {code, name, bars: [{date, close}]}。
        params: 见 DEFAULT_PARAMS，缺省用锁定参数。

    Returns:
        daily/positions/trades/metrics/quality/input_hash 等结构化事实，
        全部可 JSON 序列化，供 Go 后端直接物化。
    """
    cfg = dict(DEFAULT_PARAMS)
    if params:
        for key, value in params.items():
            if key in cfg and value is not None:
                cfg[key] = value

    top_n = int(cfg["top_n"])
    interval = int(cfg["rebalance_interval"])
    lookback = int(cfg["lookback"])
    use_timing = bool(cfg["use_timing"])
    timing_ma = int(cfg["timing_ma"])
    use_stop = bool(cfg["use_stop"])
    stop_pct = float(cfg["stop_pct"])
    cost = float(cfg["cost"])
    risk_free = float(cfg["risk_free"])

    if top_n <= 0 or interval <= 0 or lookback <= 0:
        raise ValueError("top_n / rebalance_interval / lookback 必须为正整数")
    if not assets:
        raise ValueError("assets 不能为空")

    codes = [str(a["code"]) for a in assets]
    if len(set(codes)) != len(codes):
        raise ValueError("assets 存在重复代码")
    names = [str(a.get("name") or a["code"]) for a in assets]
    list_dates = [str(a.get("list_date") or "") for a in assets]

    bench_bars = benchmark.get("bars") or []
    if not bench_bars:
        raise ValueError("benchmark.bars 不能为空")

    # 并集日期轴：仅来自真实日线，不伪造任何日期。
    date_set = {str(b["date"]) for b in bench_bars}
    for asset in assets:
        for bar in asset.get("bars") or []:
            date_set.add(str(bar["date"]))
    dates = sorted(date_set)
    n = len(dates)
    if n < 2:
        raise ValueError("有效交易日不足，无法回放")
    m = len(assets)

    def _align(bars: List[Dict[str, Any]]) -> Dict[str, float]:
        out: Dict[str, float] = {}
        for bar in bars:
            close = bar.get("close")
            if close is None:
                continue
            value = float(close)
            if value > 0:
                out[str(bar["date"])] = value
        return out

    asset_price_maps = [_align(a.get("bars") or []) for a in assets]
    bench_price_map = _align(bench_bars)

    # 上市日期：优先 assets[].list_date 显式值，否则以该资产首个真实日线推导。
    # 推导口径保证「无数据即无效」，绝不向前填充上市前价格。
    effective_list_dates: List[str] = []
    for j in range(m):
        explicit = list_dates[j]
        if explicit:
            effective_list_dates.append(explicit)
            continue
        first = min(asset_price_maps[j].keys()) if asset_price_maps[j] else ""
        effective_list_dates.append(first)

    # closes[j][t]：真实收盘价；上市前或无日线日期为 None（不填充）。
    closes: List[List[Optional[float]]] = []
    masked_pre_listing = [0] * m
    for j in range(m):
        price_map = asset_price_maps[j]
        list_date = effective_list_dates[j]
        row: List[Optional[float]] = []
        for date in dates:
            price = price_map.get(date)
            if price is not None and list_date and date < list_date:
                masked_pre_listing[j] += 1
                price = None
            row.append(price)
        closes.append(row)
    bench: List[Optional[float]] = [bench_price_map.get(d) for d in dates]

    rf_daily = (1 + risk_free) ** (1 / TRADING_DAYS_PER_YEAR) - 1
    warmup = max(lookback, timing_ma if use_timing else 0)
    is_decision = [False] * n
    for t in range(warmup, n - 1):
        if (t - warmup) % interval == 0:
            is_decision[t] = True

    weights = [0.0] * m
    basket: List[int] = []
    pending_basket: Optional[List[int]] = None
    pending_signal_date = ""
    peaks = [0.0] * m
    entry_prices = [0.0] * m
    port = 1.0
    total_cost = 0.0
    n_rebal = 0

    daily: List[Dict[str, Any]] = []
    position_rows: List[Dict[str, Any]] = []
    trades: List[Dict[str, Any]] = []
    quality_missing: List[Dict[str, Any]] = []
    blocked_events: List[Dict[str, Any]] = []
    in_market_prev = True

    bench_base: Optional[float] = None
    for value in bench:
        if value is not None and value > 0:
            bench_base = value
            break

    for t in range(n):
        date = dates[t]
        if t == 0:
            daily.append({
                "date": date, "nav": _round(port), "daily_return": 0.0,
                "bench_nav": 1.0 if bench_base else None,
                "cash_weight": 1.0, "holding_count": 0, "in_market": in_market_prev,
            })
            continue

        # 1) 当日收益：先以「旧权重」结算，再谈调仓（修正同日收益）。
        port_ret = 0.0
        weight_sum = 0.0
        for j in range(m):
            if weights[j] <= 0:
                continue
            weight_sum += weights[j]
            cur, prev = closes[j][t], closes[j][t - 1]
            if cur is not None and prev is not None and prev > 0:
                port_ret += weights[j] * (cur / prev - 1)
            else:
                # 持仓缺价：按 0 收益处理（经济上≈停牌），但计入质量报告。
                quality_missing.append({"date": date, "code": codes[j], "reason": "held_missing_price"})
        port_ret += (1 - weight_sum) * rf_daily
        port *= (1 + port_ret)
        if port < 1e-6:
            port = 1e-6

        # 2) 收盘执行：pending 篮子生效 + 风险叠加层 + 调仓成交。
        if pending_basket is not None:
            basket = pending_basket
            exec_signal_date = pending_signal_date
            pending_basket = None
        else:
            exec_signal_date = ""

        bench_close = bench[t]
        if use_timing and bench_close is not None:
            ma_value = _sma(bench, t, timing_ma)
            in_market = (ma_value is None) or (bench_close >= ma_value)
            in_market_prev = in_market
        else:
            in_market = in_market_prev

        target = [0.0] * m
        if in_market and basket:
            equal_weight = 1.0 / len(basket)
            for j in basket:
                target[j] = equal_weight

        if use_stop and in_market and basket:
            for j in basket:
                if target[j] <= 0:
                    continue
                price = closes[j][t]
                if price is None:
                    continue
                if peaks[j] <= 0 or weights[j] == 0:
                    peaks[j] = price
                elif price > peaks[j]:
                    peaks[j] = price
                if price < peaks[j] * (1 - stop_pct):
                    target[j] = 0.0

        change = sum(abs(target[j] - weights[j]) for j in range(m))
        if change > _EPS:
            executed = False
            for j in range(m):
                if abs(target[j] - weights[j]) <= _EPS:
                    continue
                price = closes[j][t]
                if price is None:
                    # 无真实价格无法成交：保持旧权重并记 blocked，不伪造价格。
                    blocked_events.append({"date": date, "code": codes[j],
                                           "reason": "missing_price_cannot_trade"})
                    target[j] = weights[j]
                    continue
                if target[j] == 0 and weights[j] > 0:
                    reason = "RS轮动·调出(信号消失)"
                    if use_timing and not in_market:
                        reason = "沪深300择时·转现金"
                    elif use_stop and peaks[j] > 0 and price < peaks[j] * (1 - stop_pct):
                        reason = "个股跟踪止损·清仓"
                    trades.append({"signal_date": exec_signal_date or date,
                                   "execution_date": date, "side": "SELL",
                                   "code": codes[j], "name": names[j],
                                   "price": _round(price), "weight": 0.0,
                                   "reason": reason})
                    executed = True
                elif target[j] > 0 and weights[j] == 0:
                    trades.append({"signal_date": exec_signal_date or date,
                                   "execution_date": date, "side": "BUY",
                                   "code": codes[j], "name": names[j],
                                   "price": _round(price), "weight": _round(target[j]),
                                   "reason": "RS轮动·建仓(信号入选)"})
                    entry_prices[j] = price
                    executed = True
            # 成本按实际成交的换手收取。
            executed_change = sum(abs(target[j] - weights[j]) for j in range(m))
            if executed and executed_change > _EPS:
                cost_amount = executed_change * port * cost
                port -= cost_amount
                total_cost += cost_amount
                n_rebal += 1
            weights = target[:]

        # 3) 收盘决策：RS 信号仅供下一交易日执行（pending）。
        if is_decision[t]:
            signals: List[float] = []
            for j in range(m):
                cur = closes[j][t]
                base = closes[j][t - lookback] if t - lookback >= 0 else None
                if cur is not None and base is not None and base > 0:
                    signals.append(cur / base - 1)
                else:
                    signals.append(_INVALID_SIGNAL)
            positive = [j for j in range(m) if signals[j] > 0]
            positive.sort(key=lambda j: signals[j], reverse=True)
            pending_basket = positive[: min(top_n, len(positive))]
            pending_signal_date = date

        holding_count = sum(1 for j in range(m) if weights[j] > 0)
        bench_nav = (bench_close / bench_base) if (bench_close is not None and bench_base) else None
        daily.append({
            "date": date, "nav": _round(port), "daily_return": _round(port_ret),
            "bench_nav": _round(bench_nav) if bench_nav is not None else None,
            "cash_weight": _round(max(0.0, 1 - sum(weights))),
            "holding_count": holding_count, "in_market": in_market,
        })
        for j in range(m):
            if weights[j] > 0:
                position_rows.append({
                    "date": date, "code": codes[j], "name": names[j],
                    "weight": _round(weights[j]),
                    "price": _round(closes[j][t]) if closes[j][t] is not None else None,
                })

    equity = [row["nav"] for row in daily if row["nav"] is not None]
    bench_nav_series = [row["bench_nav"] if row["bench_nav"] is not None else 1.0 for row in daily]
    axis = [row["date"] for row in daily]
    metrics = compute_metrics(equity, bench_nav_series, axis_dates=axis, risk_free=risk_free)

    # 质量报告：覆盖率、缺失、掩蔽、阻断，全部显式化。
    total_asset_days = 0
    covered_asset_days = 0
    per_asset = []
    for j in range(m):
        listed_dates = [d for d in dates if not effective_list_dates[j] or d >= effective_list_dates[j]]
        covered = sum(1 for d in listed_dates if asset_price_maps[j].get(d) is not None)
        total = len(listed_dates)
        total_asset_days += total
        covered_asset_days += covered
        per_asset.append({
            "code": codes[j], "name": names[j], "list_date": effective_list_dates[j],
            "expected_days": total, "actual_days": covered,
            "coverage": _round(covered / total, 4) if total else None,
            "masked_pre_listing_bars": masked_pre_listing[j],
        })
    bench_covered = sum(1 for d in dates if bench_price_map.get(d) is not None)
    warnings: List[str] = []
    if quality_missing:
        warnings.append(f"持仓缺价按 0 收益处理 {len(quality_missing)} 次")
    if blocked_events:
        warnings.append(f"因缺价无法成交 {len(blocked_events)} 次，已保持原持仓")
    if any(item["masked_pre_listing_bars"] for item in per_asset):
        warnings.append("已按上市日期掩蔽上市前价格，未做未来价格回填")

    input_hash = hashlib.sha256(json.dumps({
        "assets": [{"code": a["code"], "bars": a.get("bars") or []} for a in assets],
        "benchmark": benchmark.get("bars") or [],
        "params": cfg, "engine_version": ENGINE_VERSION,
    }, sort_keys=True, ensure_ascii=False).encode("utf-8")).hexdigest()

    return {
        "engine_key": ENGINE_KEY,
        "engine_version": ENGINE_VERSION,
        "params": cfg,
        "universe": [{"code": codes[j], "name": names[j], "list_date": effective_list_dates[j]}
                     for j in range(m)],
        "benchmark": {"code": str(benchmark.get("code") or ""),
                      "name": str(benchmark.get("name") or "")},
        "start_date": dates[0],
        "end_date": dates[-1],
        "daily": daily,
        "positions": position_rows,
        "trades": trades,
        "metrics": metrics,
        "summary": {
            "n_rebalances": n_rebal,
            "total_cost": _round(total_cost),
            "final_holding_count": sum(1 for w in weights if w > 0),
            "in_market_last": in_market_prev,
        },
        "quality": {
            "coverage": _round(covered_asset_days / total_asset_days, 4) if total_asset_days else None,
            "benchmark_coverage": _round(bench_covered / n, 4) if n else None,
            "held_missing_price_events": len(quality_missing),
            "blocked_trade_events": len(blocked_events),
            "missing": quality_missing[:500],
            "blocked": blocked_events[:200],
            "per_asset": per_asset,
            "warnings": warnings,
        },
        "input_hash": input_hash,
    }
