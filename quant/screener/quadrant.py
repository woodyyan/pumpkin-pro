"""
四象限模型 — 全市场 A 股预计算模块

Opportunity Score = 0.5 * Trend + 0.3 * Flow + 0.2 * Revision
Risk Score        = 0.4 * Volatility + 0.3 * Drawdown + 0.3 * Crowding

所有子指标先做 percentile rank (0~100)，加权组合后最终分数也在 0~100。
"""

import logging
import math
import time
import threading
from concurrent.futures import ThreadPoolExecutor, as_completed
from typing import Any, Dict, List, Optional, Tuple

import akshare as ak
import numpy as np
import pandas as pd

from screener.scanner import get_a_share_snapshot

logger = logging.getLogger(__name__)

# ── Configuration ──────────────────────────────────────────────
DAILY_LOOKBACK_DAYS = 90           # 拉取 90 天日线（覆盖 60 天回撤 + 20 天波动率）
MAX_WORKERS = 3                    # 并发拉日线的线程数（云服务器需降低避免被限流）
REQUEST_INTERVAL_MS = 200          # 每次请求后的间隔（毫秒）
SINGLE_RETRY_DELAY_MS = 500        # 单只失败后重试前的等待（毫秒）
MIN_SUCCESS_RATIO = 0.80           # 成功率 < 80% 视为整体失败
BENCHMARK_CODE = "000001"          # 上证指数

# Quadrant thresholds
OPPORTUNITY_HIGH = 70
OPPORTUNITY_LOW = 40
RISK_HIGH = 70
RISK_LOW = 40

# ── Cache ──────────────────────────────────────────────────────
_quadrant_cache_lock = threading.Lock()
_quadrant_cache_data: Optional[List[Dict[str, Any]]] = None
_quadrant_cache_ts: float = 0.0
_QUADRANT_CACHE_TTL = 6 * 3600  # 6 hours


def _fetch_daily_bars_safe(symbol: str, days: int = DAILY_LOOKBACK_DAYS) -> Optional[pd.DataFrame]:
    """拉单只股票日线，失败返回 None"""
    try:
        end_date = pd.Timestamp.today().strftime("%Y%m%d")
        start_date = (pd.Timestamp.today() - pd.Timedelta(days=days + 30)).strftime("%Y%m%d")
        df = ak.stock_zh_a_hist(
            symbol=symbol,
            period="daily",
            start_date=start_date,
            end_date=end_date,
            adjust="qfq",
        )
        if df is None or df.empty:
            return None

        # Rename columns
        col_map = {
            "日期": "date", "开盘": "open", "收盘": "close",
            "最高": "high", "最低": "low", "成交量": "volume",
            "成交额": "turnover", "换手率": "turnover_rate",
        }
        available = {k: v for k, v in col_map.items() if k in df.columns}
        df = df.rename(columns=available)

        for col in ["open", "close", "high", "low", "volume", "turnover", "turnover_rate"]:
            if col in df.columns:
                df[col] = pd.to_numeric(df[col], errors="coerce")

        if "date" in df.columns:
            df["date"] = pd.to_datetime(df["date"])
            df = df.sort_values("date").reset_index(drop=True)

        # Keep only last N trading days
        if len(df) > days:
            df = df.tail(days).reset_index(drop=True)

        return df
    except Exception as exc:
        logger.debug("Failed to fetch daily bars for %s: %s", symbol, exc)
        return None


def _fetch_benchmark_60d_return() -> float:
    """获取上证指数近 60 个交易日的收益率"""
    try:
        end_date = pd.Timestamp.today().strftime("%Y%m%d")
        start_date = (pd.Timestamp.today() - pd.Timedelta(days=120)).strftime("%Y%m%d")
        df = ak.stock_zh_index_daily_em(
            symbol="sh000001",
            start_date=start_date,
            end_date=end_date,
        )
        if df is None or df.empty or len(df) < 2:
            return 0.0

        col_map = {"date": "date", "close": "close"}
        for cn_col, en_col in [("日期", "date"), ("收盘", "close")]:
            if cn_col in df.columns:
                col_map[cn_col] = en_col

        if "收盘" in df.columns:
            df = df.rename(columns={"日期": "date", "收盘": "close"})
        df["close"] = pd.to_numeric(df["close"], errors="coerce")
        df = df.dropna(subset=["close"]).sort_values("date").reset_index(drop=True)

        if len(df) < 60:
            lookback = len(df)
        else:
            lookback = 60

        first_close = float(df.iloc[-lookback]["close"])
        last_close = float(df.iloc[-1]["close"])
        if first_close <= 0:
            return 0.0
        return (last_close / first_close - 1) * 100
    except Exception as exc:
        logger.warning("Failed to fetch benchmark 60d return: %s", exc)
        return 0.0


def _percentile_rank(series: pd.Series) -> pd.Series:
    """Rank as percentile 0~100. NaN stays NaN."""
    return series.rank(pct=True, na_option="keep") * 100


def _compute_daily_metrics(daily_df: pd.DataFrame) -> Dict[str, float]:
    """从日线 DataFrame 计算波动率、最大回撤、20日均成交额、累计换手率"""
    result: Dict[str, float] = {
        "std_20d": np.nan,
        "max_drawdown_60d": np.nan,
        "turnover_20d_avg": np.nan,
        "cumulative_turnover_20d": np.nan,
    }

    if daily_df is None or daily_df.empty or "close" not in daily_df.columns:
        return result

    closes = daily_df["close"].dropna()
    if len(closes) < 5:
        return result

    # 20-day return volatility (std of daily returns)
    returns = closes.pct_change().dropna()
    if len(returns) >= 20:
        result["std_20d"] = float(returns.tail(20).std())
    elif len(returns) >= 5:
        result["std_20d"] = float(returns.std())

    # 60-day max drawdown
    lookback_closes = closes.tail(60) if len(closes) >= 60 else closes
    rolling_max = lookback_closes.cummax()
    drawdown = (lookback_closes - rolling_max) / rolling_max
    result["max_drawdown_60d"] = abs(float(drawdown.min())) if len(drawdown) > 0 else 0.0

    # 20-day average turnover (成交额)
    if "turnover" in daily_df.columns:
        turnovers = daily_df["turnover"].dropna()
        if len(turnovers) >= 20:
            result["turnover_20d_avg"] = float(turnovers.tail(20).mean())
        elif len(turnovers) >= 5:
            result["turnover_20d_avg"] = float(turnovers.mean())

    # 20-day cumulative turnover_rate (换手率)
    if "turnover_rate" in daily_df.columns:
        tr = daily_df["turnover_rate"].dropna()
        if len(tr) >= 20:
            result["cumulative_turnover_20d"] = float(tr.tail(20).sum())
        elif len(tr) >= 5:
            result["cumulative_turnover_20d"] = float(tr.sum())

    return result


def compute_all_quadrant_scores(callback_url: Optional[str] = None) -> List[Dict[str, Any]]:
    """
    全市场 A 股四象限评分。

    1. 拉全市场快照
    2. 并发拉每只股票 90 天日线（6线程 + 50ms 间隔）
    3. 拉上证指数 60 日收益
    4. 计算所有子指标 → percentile rank → 加权组合
    5. 返回评分列表

    Returns:
        List of dicts with code, name, opportunity, risk, quadrant, sub-scores
    """
    start_time = time.time()

    # ── Step 1: 全市场快照 ──
    logger.info("[quadrant] Step 1: 拉取全市场快照...")
    snapshot_df = get_a_share_snapshot()
    if snapshot_df is None or snapshot_df.empty:
        raise RuntimeError("全市场快照数据为空")
    logger.info("[quadrant] 快照: %d 只股票", len(snapshot_df))

    # Filter: only stocks with valid code and price > 0
    snapshot_df = snapshot_df[
        snapshot_df["code"].notna()
        & (snapshot_df["price"].notna())
        & (snapshot_df["price"] > 0)
    ].copy()
    all_codes = snapshot_df["code"].tolist()
    total_stocks = len(all_codes)
    logger.info("[quadrant] 有效股票: %d 只", total_stocks)

    # ── Step 2: 并发拉日线 ──
    logger.info("[quadrant] Step 2: 并发拉取 %d 只股票日线 (workers=%d, interval=%dms)...",
                total_stocks, MAX_WORKERS, REQUEST_INTERVAL_MS)
    daily_data: Dict[str, pd.DataFrame] = {}
    failed_codes: List[str] = []

    def fetch_with_interval(code: str) -> Tuple[str, Optional[pd.DataFrame]]:
        df = _fetch_daily_bars_safe(code)
        if df is None:
            # Retry once with longer delay
            time.sleep(SINGLE_RETRY_DELAY_MS / 1000.0)
            df = _fetch_daily_bars_safe(code)
        if REQUEST_INTERVAL_MS > 0:
            time.sleep(REQUEST_INTERVAL_MS / 1000.0)
        return code, df

    with ThreadPoolExecutor(max_workers=MAX_WORKERS) as executor:
        futures = {executor.submit(fetch_with_interval, code): code for code in all_codes}
        done_count = 0
        for future in as_completed(futures):
            code = futures[future]
            try:
                result_code, result_df = future.result()
                if result_df is not None and not result_df.empty:
                    daily_data[result_code] = result_df
                else:
                    failed_codes.append(result_code)
            except Exception:
                failed_codes.append(code)

            done_count += 1
            if done_count % 500 == 0:
                logger.info("[quadrant] 日线进度: %d/%d", done_count, total_stocks)

    success_count = len(daily_data)
    success_ratio = success_count / total_stocks if total_stocks > 0 else 0
    logger.info("[quadrant] 日线完成: 成功 %d / 总 %d (%.1f%%), 失败 %d",
                success_count, total_stocks, success_ratio * 100, len(failed_codes))

    if success_ratio < MIN_SUCCESS_RATIO:
        raise RuntimeError(
            f"日线拉取成功率过低: {success_count}/{total_stocks} ({success_ratio:.1%})，"
            f"阈值 {MIN_SUCCESS_RATIO:.0%}"
        )

    # ── Step 3: 上证指数 60 日收益 ──
    logger.info("[quadrant] Step 3: 拉取上证指数 60 日收益...")
    bench_60d = _fetch_benchmark_60d_return()
    logger.info("[quadrant] 上证 60 日收益: %.2f%%", bench_60d)

    # ── Step 4: 计算子指标 ──
    logger.info("[quadrant] Step 4: 计算子指标...")

    # Merge daily metrics into snapshot
    daily_metrics_rows = []
    for code in all_codes:
        daily_df = daily_data.get(code)
        metrics = _compute_daily_metrics(daily_df)
        metrics["code"] = code
        daily_metrics_rows.append(metrics)

    daily_metrics_df = pd.DataFrame(daily_metrics_rows)
    merged = snapshot_df.merge(daily_metrics_df, on="code", how="left")

    # Compute sub-scores
    # ── Trend ──
    change_60d_rank = _percentile_rank(merged["change_pct_60d"])
    excess_return = merged["change_pct_60d"] - bench_60d
    excess_rank = _percentile_rank(excess_return)
    merged["trend"] = 0.5 * change_60d_rank + 0.5 * excess_rank

    # ── Flow ──
    volume_ratio_rank = _percentile_rank(merged["volume_ratio"])
    turnover_rate_rank = _percentile_rank(merged["turnover_rate"])
    # 成交额比 = 今日成交额 / 20日均成交额
    turnover_ratio = merged["turnover"] / merged["turnover_20d_avg"]
    turnover_ratio_rank = _percentile_rank(turnover_ratio)
    merged["flow"] = 0.4 * volume_ratio_rank + 0.3 * turnover_rate_rank + 0.3 * turnover_ratio_rank

    # ── Revision ──
    merged["revision"] = _percentile_rank(merged["profit_growth_rate"])

    # ── Volatility ──
    merged["volatility_raw"] = _percentile_rank(merged["std_20d"])

    # ── Drawdown ──
    merged["drawdown_raw"] = _percentile_rank(merged["max_drawdown_60d"])

    # ── Crowding ──
    pe_rank = _percentile_rank(merged["pe"])
    cum_turnover_rank = _percentile_rank(merged["cumulative_turnover_20d"])
    merged["crowding_raw"] = 0.5 * pe_rank + 0.5 * cum_turnover_rank

    # ── Final scores ──
    merged["opportunity"] = (
        0.5 * merged["trend"] + 0.3 * merged["flow"] + 0.2 * merged["revision"]
    )
    merged["risk"] = (
        0.4 * merged["volatility_raw"] + 0.3 * merged["drawdown_raw"] + 0.3 * merged["crowding_raw"]
    )

    # Fill NaN with 50 (neutral) for final scores
    merged["opportunity"] = merged["opportunity"].fillna(50).clip(0, 100).round(2)
    merged["risk"] = merged["risk"].fillna(50).clip(0, 100).round(2)

    # Fill NaN sub-scores with 50
    for col in ["trend", "flow", "revision", "volatility_raw", "drawdown_raw", "crowding_raw"]:
        merged[col] = merged[col].fillna(50).clip(0, 100).round(2)

    # ── Step 5: 分配象限 ──
    def assign_quadrant(row):
        opp = row["opportunity"]
        rsk = row["risk"]
        if opp > OPPORTUNITY_HIGH and rsk < RISK_LOW:
            return "机会"
        if opp > OPPORTUNITY_HIGH and rsk > RISK_HIGH:
            return "拥挤"
        if opp < OPPORTUNITY_LOW and rsk > RISK_HIGH:
            return "泡沫"
        if opp < OPPORTUNITY_LOW and rsk < RISK_LOW:
            return "防御"
        return "中性"

    merged["quadrant"] = merged.apply(assign_quadrant, axis=1)

    # Build result
    result_items = []
    for _, row in merged.iterrows():
        code = str(row.get("code", ""))
        name = str(row.get("name", "")) if pd.notna(row.get("name")) else code
        result_items.append({
            "code": code,
            "name": name,
            "opportunity": float(row["opportunity"]),
            "risk": float(row["risk"]),
            "quadrant": row["quadrant"],
            "trend": float(row["trend"]),
            "flow": float(row["flow"]),
            "revision": float(row["revision"]),
            "volatility": float(row["volatility_raw"]),
            "drawdown": float(row["drawdown_raw"]),
            "crowding": float(row["crowding_raw"]),
        })

    elapsed = time.time() - start_time
    logger.info("[quadrant] ✅ 计算完成: %d 只股票, 耗时 %.1f 秒", len(result_items), elapsed)

    # Update cache
    with _quadrant_cache_lock:
        global _quadrant_cache_data, _quadrant_cache_ts
        _quadrant_cache_data = result_items
        _quadrant_cache_ts = time.time()

    # Callback to Go backend if URL provided
    if callback_url:
        _send_callback(callback_url, result_items)

    return result_items


def get_cached_scores() -> Optional[List[Dict[str, Any]]]:
    """返回缓存的四象限评分，如果缓存过期或为空则返回 None"""
    with _quadrant_cache_lock:
        if _quadrant_cache_data is not None and (time.time() - _quadrant_cache_ts) < _QUADRANT_CACHE_TTL:
            return _quadrant_cache_data
    return None


def _send_callback(callback_url: str, items: List[Dict[str, Any]]):
    """将计算结果回调给 Go 后端"""
    import requests

    try:
        payload = {"items": items, "computed_at": pd.Timestamp.now(tz="UTC").isoformat()}
        resp = requests.post(
            callback_url,
            json=payload,
            timeout=30,
            headers={"Content-Type": "application/json"},
        )
        if resp.status_code < 200 or resp.status_code >= 300:
            logger.warning("[quadrant] 回调失败: HTTP %d, body=%s", resp.status_code, resp.text[:200])
        else:
            logger.info("[quadrant] 回调成功: HTTP %d, 写入 %d 条", resp.status_code, len(items))
    except Exception as exc:
        logger.error("[quadrant] 回调异常: %s", exc)
