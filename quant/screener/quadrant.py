"""
四象限模型 — 全市场 A 股预计算模块

Opportunity Score = 0.5 * Trend + 0.3 * Flow + 0.2 * Revision
Risk Score        = 0.4 * Volatility + 0.3 * Drawdown + 0.3 * Crowding

所有子指标先做 percentile rank (0~100)，加权组合后最终分数也在 0~100。

数据源策略：腾讯财经优先，东财 AKShare 降级。
缓存策略：本地 JSON 文件缓存日线数据，每日增量更新。
"""

import json as _json
import logging
import os
import time
import threading
from concurrent.futures import ThreadPoolExecutor, as_completed
from typing import Any, Dict, List, Optional, Tuple

import numpy as np
import pandas as pd
import requests

from screener.scanner import get_a_share_snapshot

logger = logging.getLogger(__name__)

# ── Configuration ──────────────────────────────────────────────
DAILY_LOOKBACK_DAYS = 90           # 需要的历史日线天数
MAX_WORKERS = 3                    # 并发线程数
REQUEST_INTERVAL_MS = 200          # 每次请求后的间隔（毫秒）
SINGLE_RETRY_DELAY_MS = 500        # 单只失败后重试前的等待（毫秒）
MIN_SUCCESS_RATIO = 0.80           # 成功率 < 80% 视为整体失败
FULL_REFRESH_INTERVAL_DAYS = 7     # 每 7 天强制全量刷新一次缓存
CACHE_FILE_PATH = os.path.join(os.path.dirname(os.path.dirname(__file__)), "data", "cache", "quadrant_daily_cache.json")

# Quadrant thresholds
OPPORTUNITY_HIGH = 70
OPPORTUNITY_LOW = 40
RISK_HIGH = 70
RISK_LOW = 40

# ── Result cache (in-memory, for /api/quadrant/scores) ─────────
_quadrant_cache_lock = threading.Lock()
_quadrant_cache_data: Optional[List[Dict[str, Any]]] = None
_quadrant_cache_ts: float = 0.0
_QUADRANT_CACHE_TTL = 6 * 3600  # 6 hours


# ── Daily bar cache (file-based) ──────────────────────────────

class DailyBarCache:
    """管理本地日线缓存文件。"""

    def __init__(self, file_path: str = CACHE_FILE_PATH):
        self.file_path = file_path
        self._data: Optional[Dict] = None

    def load(self) -> Dict:
        """从文件加载缓存，不存在则返回空结构。"""
        if self._data is not None:
            return self._data
        if not os.path.exists(self.file_path):
            self._data = {"version": 2, "last_full_refresh": "", "last_incremental": "", "stocks": {}}
            return self._data
        try:
            with open(self.file_path, "r", encoding="utf-8") as f:
                self._data = _json.load(f)
            logger.info("[quadrant-cache] 加载缓存: %d 只股票", len(self._data.get("stocks", {})))
            return self._data
        except Exception as exc:
            logger.warning("[quadrant-cache] 加载缓存失败: %s, 将重新创建", exc)
            self._data = {"version": 2, "last_full_refresh": "", "last_incremental": "", "stocks": {}}
            return self._data

    def save(self):
        """将缓存写回文件。"""
        if self._data is None:
            return
        try:
            os.makedirs(os.path.dirname(self.file_path), exist_ok=True)
            with open(self.file_path, "w", encoding="utf-8") as f:
                _json.dump(self._data, f, ensure_ascii=False)
            stock_count = len(self._data.get("stocks", {}))
            logger.info("[quadrant-cache] 缓存已保存: %d 只股票", stock_count)
        except Exception as exc:
            logger.error("[quadrant-cache] 保存缓存失败: %s", exc)

    def needs_full_refresh(self, force_full: bool = False) -> bool:
        """判断是否需要全量刷新。"""
        if force_full:
            return True
        data = self.load()
        if not data.get("stocks"):
            return True
        last_full = data.get("last_full_refresh", "")
        if not last_full:
            return True
        try:
            last_date = pd.Timestamp(last_full)
            days_since = (pd.Timestamp.today() - last_date).days
            return days_since >= FULL_REFRESH_INTERVAL_DAYS
        except Exception:
            return True

    def get_stock_bars(self, code: str) -> Optional[List[Dict]]:
        """获取某只股票的缓存日线。"""
        data = self.load()
        stock = data.get("stocks", {}).get(code)
        if stock is None:
            return None
        return stock.get("bars")

    def set_stock_bars(self, code: str, bars: List[Dict]):
        """更新某只股票的缓存日线。"""
        data = self.load()
        if "stocks" not in data:
            data["stocks"] = {}
        data["stocks"][code] = {
            "bars": bars,
            "updated_at": pd.Timestamp.today().strftime("%Y-%m-%d"),
        }

    def merge_incremental(self, code: str, new_bars: List[Dict]):
        """将增量日线追加到缓存，去重 + 裁剪。"""
        existing = self.get_stock_bars(code) or []
        existing_dates = {b["date"] for b in existing}
        for bar in new_bars:
            if bar["date"] not in existing_dates:
                existing.append(bar)
                existing_dates.add(bar["date"])
        # Sort by date and keep only last DAILY_LOOKBACK_DAYS
        existing.sort(key=lambda b: b["date"])
        if len(existing) > DAILY_LOOKBACK_DAYS:
            existing = existing[-DAILY_LOOKBACK_DAYS:]
        self.set_stock_bars(code, existing)

    def mark_full_refresh(self):
        data = self.load()
        data["last_full_refresh"] = pd.Timestamp.today().strftime("%Y-%m-%d")
        data["last_incremental"] = pd.Timestamp.today().strftime("%Y-%m-%d")

    def mark_incremental(self):
        data = self.load()
        data["last_incremental"] = pd.Timestamp.today().strftime("%Y-%m-%d")

    def bars_to_dataframe(self, code: str) -> Optional[pd.DataFrame]:
        """将缓存日线转为 DataFrame。"""
        bars = self.get_stock_bars(code)
        if not bars:
            return None
        df = pd.DataFrame(bars)
        df["date"] = pd.to_datetime(df["date"])
        for col in ["open", "close", "high", "low", "volume"]:
            if col in df.columns:
                df[col] = pd.to_numeric(df[col], errors="coerce")
        df = df.sort_values("date").reset_index(drop=True)
        return df


# ── Data source: Tencent Finance (primary) ─────────────────────

_QQ_DAILY_HEADERS = {
    "User-Agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 "
                  "(KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36",
    "Referer": "https://stockapp.finance.qq.com/",
}


def _code_to_qq_daily(code: str) -> str:
    c = str(code).zfill(6)
    return f"sh{c}" if c.startswith(("6", "9")) else f"sz{c}"


def _fetch_bars_tencent(symbol: str, days: int) -> Optional[List[Dict]]:
    """通过腾讯财经日线接口拉取前复权日线。"""
    try:
        qq_code = _code_to_qq_daily(symbol)
        end_date = pd.Timestamp.today().strftime("%Y-%m-%d")
        start_date = (pd.Timestamp.today() - pd.Timedelta(days=days + 30)).strftime("%Y-%m-%d")

        url = (
            f"https://web.ifzq.gtimg.cn/appstock/app/fqkline/get"
            f"?param={qq_code},day,{start_date},{end_date},{days + 30},qfq"
        )
        resp = requests.get(url, headers=_QQ_DAILY_HEADERS, timeout=15)
        resp.raise_for_status()
        data = resp.json()

        stock_data = data.get("data", {}).get(qq_code, {})
        klines = stock_data.get("qfqday") or stock_data.get("day") or []
        if not klines:
            return None

        rows = []
        for k in klines:
            if len(k) < 6:
                continue
            rows.append({
                "date": k[0],
                "open": float(k[1]),
                "close": float(k[2]),
                "high": float(k[3]),
                "low": float(k[4]),
                "volume": float(k[5]),
            })
        return rows if rows else None
    except Exception as exc:
        logger.debug("[tencent] fetch failed for %s: %s", symbol, exc)
        return None


# ── Data source: AKShare / Eastmoney (fallback) ────────────────

def _fetch_bars_akshare(symbol: str, days: int) -> Optional[List[Dict]]:
    """通过 AKShare 东财接口拉取日线（降级数据源）。"""
    try:
        import akshare as ak
        end_date = pd.Timestamp.today().strftime("%Y%m%d")
        start_date = (pd.Timestamp.today() - pd.Timedelta(days=days + 30)).strftime("%Y%m%d")
        df = ak.stock_zh_a_hist(
            symbol=symbol, period="daily",
            start_date=start_date, end_date=end_date, adjust="qfq",
        )
        if df is None or df.empty:
            return None

        col_map = {
            "日期": "date", "开盘": "open", "收盘": "close",
            "最高": "high", "最低": "low", "成交量": "volume",
            "换手率": "turnover_rate",
        }
        available = {k: v for k, v in col_map.items() if k in df.columns}
        df = df.rename(columns=available)

        rows = []
        for _, row in df.iterrows():
            try:
                item = {
                    "date": str(row["date"])[:10],
                    "open": float(row["open"]),
                    "close": float(row["close"]),
                    "high": float(row["high"]),
                    "low": float(row["low"]),
                    "volume": float(row["volume"]),
                }
                if "turnover_rate" in row.index and pd.notna(row["turnover_rate"]):
                    item["turnover_rate"] = float(row["turnover_rate"])
                rows.append(item)
            except (ValueError, KeyError):
                continue
        return rows if rows else None
    except Exception as exc:
        logger.debug("[akshare] fetch failed for %s: %s", symbol, exc)
        return None


# ── Dual-source fetch with fallback ────────────────────────────

def _fetch_daily_bars(symbol: str, days: int = DAILY_LOOKBACK_DAYS) -> Optional[List[Dict]]:
    """腾讯优先，东财降级。"""
    bars = _fetch_bars_tencent(symbol, days)
    if bars:
        return bars
    # Fallback to AKShare
    time.sleep(SINGLE_RETRY_DELAY_MS / 1000.0)
    bars = _fetch_bars_akshare(symbol, days)
    return bars


def _fetch_benchmark_60d_return() -> float:
    """通过腾讯财经获取上证指数近 60 个交易日的收益率。"""
    try:
        end_date = pd.Timestamp.today().strftime("%Y-%m-%d")
        start_date = (pd.Timestamp.today() - pd.Timedelta(days=120)).strftime("%Y-%m-%d")
        url = (
            f"https://web.ifzq.gtimg.cn/appstock/app/fqkline/get"
            f"?param=sh000001,day,{start_date},{end_date},90,"
        )
        resp = requests.get(url, headers=_QQ_DAILY_HEADERS, timeout=15)
        resp.raise_for_status()
        data = resp.json()
        stock_data = data.get("data", {}).get("sh000001", {})
        klines = stock_data.get("day") or stock_data.get("qfqday") or []
        if len(klines) < 2:
            return 0.0
        closes = [float(k[2]) for k in klines if len(k) >= 3 and float(k[2]) > 0]
        if len(closes) < 2:
            return 0.0
        lookback = min(60, len(closes))
        return (closes[-1] / closes[-lookback] - 1) * 100
    except Exception as exc:
        logger.warning("Failed to fetch benchmark 60d return: %s", exc)
        return 0.0


# ── Metrics computation ────────────────────────────────────────

def _percentile_rank(series: pd.Series) -> pd.Series:
    return series.rank(pct=True, na_option="keep") * 100


def _compute_daily_metrics(daily_df: pd.DataFrame) -> Dict[str, float]:
    result: Dict[str, float] = {
        "std_20d": np.nan,
        "max_drawdown_60d": np.nan,
        "turnover_20d_avg": np.nan,
        "cumulative_turnover_20d": np.nan,
        "change_pct_60d_calc": np.nan,
        "volume_ratio_calc": np.nan,
    }
    if daily_df is None or daily_df.empty or "close" not in daily_df.columns:
        return result
    closes = daily_df["close"].dropna()
    if len(closes) < 5:
        return result

    returns = closes.pct_change().dropna()
    if len(returns) >= 20:
        result["std_20d"] = float(returns.tail(20).std())
    elif len(returns) >= 5:
        result["std_20d"] = float(returns.std())

    lookback_closes = closes.tail(60) if len(closes) >= 60 else closes
    rolling_max = lookback_closes.cummax()
    drawdown = (lookback_closes - rolling_max) / rolling_max
    result["max_drawdown_60d"] = abs(float(drawdown.min())) if len(drawdown) > 0 else 0.0

    # 60 日涨跌幅（从日线自算，不依赖快照源）
    if len(closes) >= 60:
        result["change_pct_60d_calc"] = (closes.iloc[-1] / closes.iloc[-60] - 1) * 100
    elif len(closes) >= 10:
        result["change_pct_60d_calc"] = (closes.iloc[-1] / closes.iloc[0] - 1) * 100

    # 简化量比 = 今日成交量 / 近 5 日平均成交量
    vol_col = "volume" if "volume" in daily_df.columns else None
    if vol_col:
        volumes = daily_df[vol_col].dropna()
        if len(volumes) >= 6:
            today_vol = volumes.iloc[-1]
            avg_5d = volumes.iloc[-6:-1].mean()
            if avg_5d > 0:
                result["volume_ratio_calc"] = float(today_vol / avg_5d)

    turnover_col = "turnover" if "turnover" in daily_df.columns else ("volume" if "volume" in daily_df.columns else None)
    if turnover_col:
        turnovers = daily_df[turnover_col].dropna()
        if len(turnovers) >= 20:
            result["turnover_20d_avg"] = float(turnovers.tail(20).mean())
        elif len(turnovers) >= 5:
            result["turnover_20d_avg"] = float(turnovers.mean())

    if "turnover_rate" in daily_df.columns:
        tr = daily_df["turnover_rate"].dropna()
        if len(tr) >= 20:
            result["cumulative_turnover_20d"] = float(tr.tail(20).sum())
        elif len(tr) >= 5:
            result["cumulative_turnover_20d"] = float(tr.sum())

    return result


# ── Main computation ───────────────────────────────────────────

def compute_all_quadrant_scores(
    callback_url: Optional[str] = None,
    force_full: bool = False,
) -> List[Dict[str, Any]]:
    """
    全市场 A 股四象限评分。

    支持两种模式：
    - 全量刷新：首次运行 / 缓存过期 / force_full=True
    - 增量更新：有缓存时只拉最新 2 天日线，追加到缓存

    Returns:
        List of dicts with code, name, opportunity, risk, quadrant, sub-scores
    """
    start_time = time.time()
    cache = DailyBarCache()

    # ── Step 1: 全市场快照 ──
    logger.info("[quadrant] Step 1: 拉取全市场快照...")
    snapshot_df = get_a_share_snapshot()
    if snapshot_df is None or snapshot_df.empty:
        raise RuntimeError("全市场快照数据为空")

    snapshot_df = snapshot_df[
        snapshot_df["code"].notna()
        & (snapshot_df["price"].notna())
        & (snapshot_df["price"] > 0)
    ].copy()
    all_codes = snapshot_df["code"].tolist()
    total_stocks = len(all_codes)
    logger.info("[quadrant] 有效股票: %d 只", total_stocks)

    # ── Step 2: 决定全量 vs 增量 ──
    is_full = cache.needs_full_refresh(force_full=force_full)
    if is_full:
        fetch_days = DAILY_LOOKBACK_DAYS
        logger.info("[quadrant] Step 2: 全量刷新模式 (拉取 %d 天日线)...", fetch_days)
    else:
        fetch_days = 3  # 只拉最近 3 天（覆盖周末 + 当天）
        logger.info("[quadrant] Step 2: 增量更新模式 (拉取 %d 天日线)...", fetch_days)

    # ── Step 3: 并发拉日线 ──
    logger.info("[quadrant] 并发拉取 %d 只股票 (workers=%d, interval=%dms)...",
                total_stocks, MAX_WORKERS, REQUEST_INTERVAL_MS)
    success_count = 0
    failed_codes: List[str] = []

    def fetch_with_interval(code: str) -> Tuple[str, Optional[List[Dict]]]:
        bars = _fetch_daily_bars(code, fetch_days)
        if REQUEST_INTERVAL_MS > 0:
            time.sleep(REQUEST_INTERVAL_MS / 1000.0)
        return code, bars

    with ThreadPoolExecutor(max_workers=MAX_WORKERS) as executor:
        futures = {executor.submit(fetch_with_interval, code): code for code in all_codes}
        done_count = 0
        for future in as_completed(futures):
            code = futures[future]
            try:
                result_code, result_bars = future.result()
                if result_bars:
                    if is_full:
                        cache.set_stock_bars(result_code, result_bars)
                    else:
                        cache.merge_incremental(result_code, result_bars)
                    success_count += 1
                else:
                    failed_codes.append(result_code)
            except Exception:
                failed_codes.append(code)

            done_count += 1
            if done_count % 500 == 0:
                logger.info("[quadrant] 日线进度: %d/%d (成功 %d)", done_count, total_stocks, success_count)

    fetch_ratio = success_count / total_stocks if total_stocks > 0 else 0
    logger.info("[quadrant] 日线完成: 成功 %d / 总 %d (%.1f%%)",
                success_count, total_stocks, fetch_ratio * 100)

    # For full mode, check success ratio strictly
    if is_full and fetch_ratio < MIN_SUCCESS_RATIO:
        raise RuntimeError(
            f"日线拉取成功率过低: {success_count}/{total_stocks} ({fetch_ratio:.1%})，"
            f"阈值 {MIN_SUCCESS_RATIO:.0%}"
        )

    # For incremental mode, even if many fail, we still have cached data
    # Count how many stocks have usable cached data
    if not is_full:
        cached_count = sum(1 for code in all_codes if cache.get_stock_bars(code))
        cached_ratio = cached_count / total_stocks if total_stocks > 0 else 0
        logger.info("[quadrant] 缓存覆盖: %d / %d (%.1f%%)", cached_count, total_stocks, cached_ratio * 100)
        if cached_ratio < MIN_SUCCESS_RATIO:
            logger.warning("[quadrant] 缓存覆盖率不足，尝试全量刷新...")
            # Fallback: trigger full refresh
            return compute_all_quadrant_scores(callback_url=callback_url, force_full=True)

    # Update cache metadata
    if is_full:
        cache.mark_full_refresh()
    else:
        cache.mark_incremental()
    cache.save()

    # ── Step 4: 上证指数 60 日收益 ──
    logger.info("[quadrant] Step 4: 拉取上证指数 60 日收益...")
    bench_60d = _fetch_benchmark_60d_return()
    logger.info("[quadrant] 上证 60 日收益: %.2f%%", bench_60d)

    # ── Step 4.5: 将快照 turnover_rate 注入日线缓存 ──
    # 快照（东财/腾讯）都包含当天换手率，但日线缓存（腾讯源）缺少该字段。
    # 注入后 _compute_daily_metrics 能计算 cumulative_turnover_20d。
    injected_count = 0
    for _, row in snapshot_df.iterrows():
        code = row.get("code")
        tr = row.get("turnover_rate")
        if code and pd.notna(tr) and tr > 0:
            bars = cache.get_stock_bars(code)
            if bars and len(bars) > 0:
                bars[-1]["turnover_rate"] = float(tr)
                injected_count += 1
    logger.info("[quadrant] 快照换手率注入日线缓存: %d 只股票", injected_count)

    # ── Step 5: 计算子指标 ──
    logger.info("[quadrant] Step 5: 计算子指标...")

    daily_metrics_rows = []
    for code in all_codes:
        daily_df = cache.bars_to_dataframe(code)
        metrics = _compute_daily_metrics(daily_df)
        metrics["code"] = code
        daily_metrics_rows.append(metrics)

    daily_metrics_df = pd.DataFrame(daily_metrics_rows)
    merged = snapshot_df.merge(daily_metrics_df, on="code", how="left")

    # ── Diagnostic: log non-null rates for all key columns ──
    total = len(merged)
    diag_cols = ["std_20d", "max_drawdown_60d", "turnover_20d_avg", "cumulative_turnover_20d",
                 "change_pct_60d_calc", "volume_ratio_calc", "change_pct_60d", "volume_ratio",
                 "turnover_rate", "turnover", "pe", "profit_growth_rate"]
    for col in diag_cols:
        if col in merged.columns:
            valid = int(merged[col].notna().sum())
            logger.info("[quadrant-diag] %s: %d/%d (%.1f%%)", col, valid, total, valid/total*100 if total else 0)
        else:
            logger.info("[quadrant-diag] %s: COLUMN MISSING", col)

    # Also check daily_metrics_df independently
    dm_total = len(daily_metrics_df)
    for col in ["std_20d", "max_drawdown_60d", "change_pct_60d_calc", "volume_ratio_calc"]:
        if col in daily_metrics_df.columns:
            valid = int(daily_metrics_df[col].notna().sum())
            logger.info("[quadrant-diag] daily_metrics_df.%s: %d/%d (%.1f%%)", col, valid, dm_total, valid/dm_total*100 if dm_total else 0)

    # ── Backfill missing snapshot fields from daily-bar calculations ──
    # change_pct_60d: prefer snapshot (东财), fallback to daily-bar calc
    if "change_pct_60d" in merged.columns:
        merged["change_pct_60d"] = merged["change_pct_60d"].fillna(merged["change_pct_60d_calc"])
    else:
        merged["change_pct_60d"] = merged["change_pct_60d_calc"]
    # volume_ratio: prefer snapshot (东财), fallback to daily-bar simplified calc
    if "volume_ratio" in merged.columns:
        merged["volume_ratio"] = merged["volume_ratio"].fillna(merged["volume_ratio_calc"])
    else:
        merged["volume_ratio"] = merged["volume_ratio_calc"]

    backfill_60d = int(merged["change_pct_60d"].notna().sum())
    backfill_vr = int(merged["volume_ratio"].notna().sum())
    logger.info("[quadrant] 数据补齐: change_pct_60d 有效 %d/%d, volume_ratio 有效 %d/%d",
                backfill_60d, len(merged), backfill_vr, len(merged))

    # ── Trend (NaN-tolerant) ──
    change_60d_rank = _percentile_rank(merged["change_pct_60d"]).fillna(50)
    excess_return = merged["change_pct_60d"] - bench_60d
    excess_rank = _percentile_rank(excess_return).fillna(50)
    merged["trend"] = 0.5 * change_60d_rank + 0.5 * excess_rank

    # ── Flow (NaN-tolerant) ──
    volume_ratio_rank = _percentile_rank(merged["volume_ratio"]).fillna(50)
    turnover_rate_rank = _percentile_rank(merged["turnover_rate"]).fillna(50)
    turnover_ratio = merged["turnover"] / merged["turnover_20d_avg"]
    turnover_ratio_rank = _percentile_rank(turnover_ratio).fillna(50)
    merged["flow"] = 0.4 * volume_ratio_rank + 0.3 * turnover_rate_rank + 0.3 * turnover_ratio_rank

    # ── Revision (NaN-tolerant) ──
    merged["revision"] = _percentile_rank(merged["profit_growth_rate"]).fillna(50)

    # ── Volatility (NaN-tolerant) ──
    merged["volatility_raw"] = _percentile_rank(merged["std_20d"]).fillna(50)

    # ── Drawdown (NaN-tolerant) ──
    merged["drawdown_raw"] = _percentile_rank(merged["max_drawdown_60d"]).fillna(50)

    # ── Crowding ──
    # cumulative_turnover_20d requires 5+ days of turnover_rate in daily bars,
    # which Tencent source doesn't provide. Use pe_rank alone as fallback.
    pe_rank = _percentile_rank(merged["pe"])
    cum_turnover_rank = _percentile_rank(merged["cumulative_turnover_20d"])
    has_cum_turnover = merged["cumulative_turnover_20d"].notna()
    merged["crowding_raw"] = pd.Series(np.where(
        has_cum_turnover,
        0.5 * pe_rank + 0.5 * cum_turnover_rank,
        pe_rank,  # fallback: crowding = PE rank only
    ), index=merged.index)
    logger.info("[quadrant] Crowding: %d 只用 PE+换手率, %d 只仅用 PE",
                int(has_cum_turnover.sum()), int((~has_cum_turnover).sum()))

    # ── Final scores (NaN-tolerant: fillna sub-scores with 50 before combining) ──
    v_raw = merged["volatility_raw"].fillna(50)
    d_raw = merged["drawdown_raw"].fillna(50)
    c_raw = merged["crowding_raw"].fillna(50)
    t_score = merged["trend"].fillna(50)
    f_score = merged["flow"].fillna(50)
    r_score = merged["revision"].fillna(50)

    merged["opportunity"] = 0.5 * t_score + 0.3 * f_score + 0.2 * r_score
    merged["risk"] = 0.4 * v_raw + 0.3 * d_raw + 0.3 * c_raw

    # ── Diagnostic: sub-score and final score stats ──
    for col in ["trend", "flow", "revision", "volatility_raw", "drawdown_raw", "crowding_raw", "opportunity", "risk"]:
        s = merged[col]
        valid = int(s.notna().sum())
        if valid > 0:
            logger.info("[quadrant-diag] %s: valid=%d/%d, min=%.2f, max=%.2f, mean=%.2f, std=%.2f",
                        col, valid, len(merged), s.min(), s.max(), s.mean(), s.std())
        else:
            logger.info("[quadrant-diag] %s: ALL NaN (%d rows)", col, len(merged))

    # ── Re-normalize: percentile rank the final scores to counteract
    #    variance collapse from multi-layer weighted averaging ──
    merged["opportunity"] = _percentile_rank(merged["opportunity"].fillna(50))
    merged["risk"] = _percentile_rank(merged["risk"].fillna(50))

    merged["opportunity"] = merged["opportunity"].fillna(50).clip(0, 100).round(2)
    merged["risk"] = merged["risk"].fillna(50).clip(0, 100).round(2)
    for col in ["trend", "flow", "revision", "volatility_raw", "drawdown_raw", "crowding_raw"]:
        merged[col] = merged[col].fillna(50).clip(0, 100).round(2)

    # ── Step 6: 分配象限 ──
    def assign_quadrant(row):
        opp, rsk = row["opportunity"], row["risk"]
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
    mode_label = "全量" if is_full else "增量"
    logger.info("[quadrant] ✅ 计算完成 (%s): %d 只股票, 耗时 %.1f 秒", mode_label, len(result_items), elapsed)

    # ── Build structured compute report ──
    quadrant_counts = {}
    for item in result_items:
        q = item["quadrant"]
        quadrant_counts[q] = quadrant_counts.get(q, 0) + 1

    data_quality = {}
    for col in diag_cols:
        if col in merged.columns:
            valid = int(merged[col].notna().sum())
            data_quality[col] = round(valid / total * 100, 1) if total else 0

    score_stats = {}
    for col in ["opportunity", "risk"]:
        s = merged[col]
        if s.notna().sum() > 0:
            score_stats[col] = {
                "min": round(float(s.min()), 2),
                "max": round(float(s.max()), 2),
                "mean": round(float(s.mean()), 2),
                "std": round(float(s.std()), 2),
            }

    compute_report = {
        "computed_at": pd.Timestamp.now(tz="UTC").isoformat(),
        "mode": mode_label,
        "duration_seconds": round(elapsed, 1),
        "stock_count": len(result_items),
        "daily_bars": {
            "success": success_count,
            "failed": len(failed_codes),
            "total": total_stocks,
        },
        "data_quality": data_quality,
        "score_distribution": score_stats,
        "quadrant_counts": quadrant_counts,
        "status": "success",
        "error": "",
    }
    logger.info("[quadrant] 计算报告: %s", _json.dumps(compute_report, ensure_ascii=False))

    # Update in-memory cache
    with _quadrant_cache_lock:
        global _quadrant_cache_data, _quadrant_cache_ts
        _quadrant_cache_data = result_items
        _quadrant_cache_ts = time.time()

    # Callback to Go backend (include report)
    if callback_url:
        _send_callback(callback_url, result_items, compute_report)

    return result_items


def get_cached_scores() -> Optional[List[Dict[str, Any]]]:
    with _quadrant_cache_lock:
        if _quadrant_cache_data is not None and (time.time() - _quadrant_cache_ts) < _QUADRANT_CACHE_TTL:
            return _quadrant_cache_data
    return None


def _send_callback(callback_url: str, items: List[Dict[str, Any]], report: Optional[Dict] = None):
    try:
        payload = {
            "items": items,
            "computed_at": pd.Timestamp.now(tz="UTC").isoformat(),
        }
        if report:
            payload["report"] = report
        resp = requests.post(
            callback_url, json=payload, timeout=60,
            headers={"Content-Type": "application/json"},
        )
        if resp.status_code < 200 or resp.status_code >= 300:
            logger.warning("[quadrant] 回调失败: HTTP %d, body=%s", resp.status_code, resp.text[:200])
        else:
            logger.info("[quadrant] 回调成功: HTTP %d, 写入 %d 条", resp.status_code, len(items))
    except Exception as exc:
        logger.error("[quadrant] 回调异常: %s", exc)
