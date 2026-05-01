"""
四象限模型 — 全市场预计算模块（支持 A 股 / 港股）

Opportunity Score = 0.4 * Trend + 0.25 * Flow + 0.15 * Revision + 0.20 * Liquidity
Risk Score        = 0.4 * Volatility + 0.3 * Drawdown + 0.3 * Crowding

所有子指标先做 percentile rank (0~100)，加权组合后最终分数也在 0~100。
Liquidity = percentile_rank(近5日均成交额)，用于衡量标的可交易性。

数据源策略：腾讯财经优先，东财 AKShare 降级。
缓存策略：本地 JSON 文件缓存日线数据，每日增量更新。

支持 exchange 参数：
- "SSE"/"SZSE"（A 股）— 默认，使用上证指数做基准
- "HKEX"（港股）— 使用恒生指数做基准
"""

import json as _json
import logging
import os
import sqlite3
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
_CACHE_DIR = os.path.join(os.path.dirname(os.path.dirname(__file__)), "data", "cache")
CACHE_DB_PATH = os.path.join(_CACHE_DIR, "quadrant_cache.db")
LEGACY_JSON_PATH = os.path.join(_CACHE_DIR, "quadrant_daily_cache.json")

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


# ── Daily bar cache (SQLite-based) ────────────────────────────

class DailyBarCache:
    """管理本地日线缓存，存储在 SQLite 中。"""

    # SQLite 连接重试次数和间隔（秒）
    _CONNECT_RETRIES = 3
    _CONNECT_RETRY_DELAY = 1

    def __init__(self, db_path: str = CACHE_DB_PATH):
        self.db_path = db_path
        self._conn: Optional[sqlite3.Connection] = None
        self._ensure_db()
        self._maybe_migrate_from_json()

    def _ensure_db(self):
        """创建/打开 SQLite 数据库，带自动恢复能力。

        遇到磁盘 I/O 错误时：
        1. 重试连接（最多 N 次）
        2. 如果仍失败，尝试删除损坏的数据库文件并重建
        """
        os.makedirs(os.path.dirname(self.db_path), exist_ok=True)

        last_exc = None
        for attempt in range(1, self._CONNECT_RETRIES + 1):
            try:
                self._conn = sqlite3.connect(self.db_path, timeout=30)
                self._conn.execute("PRAGMA journal_mode=WAL")
                self._conn.execute("PRAGMA synchronous=NORMAL")
                # 验证连接可用性——执行一个简单查询
                self._conn.execute("SELECT 1")
                break
            except (sqlite3.OperationalError, sqlite3.DatabaseError, OSError) as exc:
                last_exc = exc
                self._conn = None  # ← 清理失效连接，确保后续判断准确
                logger.warning(
                    "[quadrant-cache] SQLite 连接失败 (第 %d/%d 次): %s",
                    attempt, self._CONNECT_RETRIES, exc,
                )
                if attempt < self._CONNECT_RETRIES:
                    time.sleep(self._CONNECT_RETRY_DELAY)
                    continue

        if self._conn is None or not self._is_connection_alive():
            # 所有重试都失败了 → 尝试删除损坏的文件并重建
            logger.error(
                "[quadrant-cache] SQLite 连接持续失败 (%s)，尝试删除损坏的数据库文件并重建",
                last_exc,
            )
            try:
                self._delete_corrupted_files(db_path=self.db_path)
                # 短暂等待确保文件系统完成清理（尤其 CI / NFS 环境）
                time.sleep(0.5)
                # 删除后重新连接（带轻量重试，应对文件系统延迟）
                self._conn = None
                for _rebuild_attempt in range(3):
                    try:
                        self._conn = sqlite3.connect(self.db_path, timeout=30)
                        self._conn.execute("PRAGMA journal_mode=WAL")
                        self._conn.execute("PRAGMA synchronous=NORMAL")
                        self._conn.execute("SELECT 1")
                        break
                    except (sqlite3.DatabaseError, sqlite3.OperationalError):
                        self._conn = None
                        time.sleep(0.3)
                if self._conn is None:
                    raise RuntimeError(f"重建后仍无法连接数据库: {self.db_path}")
                logger.info("[quadrant-cache] ✅ 数据库已重建: %s", self.db_path)
            except Exception as rebuild_exc:
                logger.error("[quadrant-cache] ❌ 数据库重建也失败了: %s", rebuild_exc)
                raise

        # ── 建表（安全网：如果连接因文件系统延迟而不稳定，重试一次）──
        for _schema_attempt in range(2):
            try:
                self._init_schema()
                return
            except sqlite3.DatabaseError:
                # 连接可能已经失效，尝试重新打开
                self._conn.close() if self._conn else None
                self._conn = None
                time.sleep(0.3)
                self._conn = sqlite3.connect(self.db_path, timeout=30)
                self._conn.execute("PRAGMA journal_mode=WAL")
                self._conn.execute("PRAGMA synchronous=NORMAL")

        # 如果两次都失败，抛出最后的异常
        self._init_schema()

    def _init_schema(self):
        """创建必要的表结构。"""
        self._conn.execute("""
            CREATE TABLE IF NOT EXISTS daily_bars (
                code TEXT NOT NULL,
                date TEXT NOT NULL,
                open REAL NOT NULL,
                close REAL NOT NULL,
                high REAL NOT NULL,
                low REAL NOT NULL,
                volume REAL NOT NULL DEFAULT 0,
                turnover_rate REAL,
                PRIMARY KEY (code, date)
            )
        """)
        self._conn.execute("""
            CREATE TABLE IF NOT EXISTS cache_meta (
                key TEXT PRIMARY KEY,
                value TEXT NOT NULL
            )
        """)
        self._conn.commit()

    def _is_connection_alive(self) -> bool:
        """快速检查连接是否可用。"""
        if self._conn is None:
            return False
        try:
            self._conn.execute("SELECT 1")
            return True
        except Exception:
            return False

    @staticmethod
    def _delete_corrupted_files(db_path=None):
        """删除可能损坏的 SQLite 数据库及其 WAL/SHM 文件。"""
        if db_path is None:
            db_path = CACHE_DB_PATH
        import glob as _glob
        base = db_path.replace(".db", "")
        patterns = [db_path, f"{base}.db-wal", f"{base}.db-shm", f"{base}.db-journal"]
        for p in patterns:
            for f in _glob.glob(p):
                try:
                    os.remove(f)
                    logger.info("[quadrant-cache] 已删除损坏文件: %s", f)
                except OSError:
                    pass  # 忽略删除失败的次要错误

    def _maybe_migrate_from_json(self):
        """首次启动时，如果旧 JSON 缓存存在且 SQLite 为空，自动迁移。"""
        if not os.path.exists(LEGACY_JSON_PATH):
            return
        count = self._conn.execute("SELECT COUNT(*) FROM daily_bars").fetchone()[0]
        if count > 0:
            return  # SQLite already has data, skip migration
        try:
            logger.info("[quadrant-cache] 检测到旧 JSON 缓存，开始迁移到 SQLite...")
            with open(LEGACY_JSON_PATH, "r", encoding="utf-8") as f:
                data = _json.load(f)
            stocks = data.get("stocks", {})
            if not stocks:
                logger.info("[quadrant-cache] 旧缓存为空，跳过迁移")
                return
            rows = []
            for code, info in stocks.items():
                for bar in info.get("bars", []):
                    rows.append((
                        code, bar.get("date", ""),
                        float(bar.get("open", 0)), float(bar.get("close", 0)),
                        float(bar.get("high", 0)), float(bar.get("low", 0)),
                        float(bar.get("volume", 0)),
                        float(bar["turnover_rate"]) if bar.get("turnover_rate") is not None else None,
                    ))
            self._conn.executemany(
                "INSERT OR REPLACE INTO daily_bars (code, date, open, close, high, low, volume, turnover_rate) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
                rows,
            )
            # Migrate meta
            for key in ("last_full_refresh", "last_incremental"):
                val = data.get(key, "")
                if val:
                    self._conn.execute("INSERT OR REPLACE INTO cache_meta (key, value) VALUES (?, ?)", (key, val))
            self._conn.commit()
            logger.info("[quadrant-cache] ✅ 迁移完成: %d 只股票, %d 条日线", len(stocks), len(rows))
        except Exception as exc:
            logger.error("[quadrant-cache] 迁移失败: %s", exc)

    def _get_meta(self, key: str) -> str:
        row = self._conn.execute("SELECT value FROM cache_meta WHERE key = ?", (key,)).fetchone()
        return row[0] if row else ""

    def _set_meta(self, key: str, value: str):
        self._conn.execute("INSERT OR REPLACE INTO cache_meta (key, value) VALUES (?, ?)", (key, value))
        self._conn.commit()

    def save(self):
        """提交所有待写入的数据。"""
        if self._conn:
            self._conn.commit()
            count = self._conn.execute("SELECT COUNT(DISTINCT code) FROM daily_bars").fetchone()[0]
            logger.info("[quadrant-cache] 缓存已保存: %d 只股票", count)

    def needs_full_refresh(self, force_full: bool = False) -> bool:
        if force_full:
            return True
        count = self._conn.execute("SELECT COUNT(*) FROM daily_bars").fetchone()[0]
        if count == 0:
            return True
        last_full = self._get_meta("last_full_refresh")
        if not last_full:
            return True
        try:
            last_date = pd.Timestamp(last_full)
            days_since = (pd.Timestamp.today() - last_date).days
            return days_since >= FULL_REFRESH_INTERVAL_DAYS
        except Exception:
            return True

    def get_stock_bars(self, code: str) -> Optional[List[Dict]]:
        rows = self._conn.execute(
            "SELECT date, open, close, high, low, volume, turnover_rate FROM daily_bars WHERE code = ? ORDER BY date",
            (code,),
        ).fetchall()
        if not rows:
            return None
        bars = []
        for r in rows:
            bar = {"date": r[0], "open": r[1], "close": r[2], "high": r[3], "low": r[4], "volume": r[5]}
            if r[6] is not None:
                bar["turnover_rate"] = r[6]
            bars.append(bar)
        return bars

    def set_stock_bars(self, code: str, bars: List[Dict]):
        self._conn.execute("DELETE FROM daily_bars WHERE code = ?", (code,))
        rows = []
        for bar in bars:
            rows.append((
                code, bar.get("date", ""),
                float(bar.get("open", 0)), float(bar.get("close", 0)),
                float(bar.get("high", 0)), float(bar.get("low", 0)),
                float(bar.get("volume", 0)),
                float(bar["turnover_rate"]) if bar.get("turnover_rate") is not None else None,
            ))
        self._conn.executemany(
            "INSERT OR REPLACE INTO daily_bars (code, date, open, close, high, low, volume, turnover_rate) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
            rows,
        )

    def merge_incremental(self, code: str, new_bars: List[Dict]):
        existing = self.get_stock_bars(code) or []
        existing_dates = {b["date"] for b in existing}
        for bar in new_bars:
            if bar["date"] not in existing_dates:
                existing.append(bar)
                existing_dates.add(bar["date"])
        existing.sort(key=lambda b: b["date"])
        if len(existing) > DAILY_LOOKBACK_DAYS:
            existing = existing[-DAILY_LOOKBACK_DAYS:]
        self.set_stock_bars(code, existing)

    def mark_full_refresh(self):
        today = pd.Timestamp.today().strftime("%Y-%m-%d")
        self._set_meta("last_full_refresh", today)
        self._set_meta("last_incremental", today)

    def mark_incremental(self):
        self._set_meta("last_incremental", pd.Timestamp.today().strftime("%Y-%m-%d"))

    def bars_to_dataframe(self, code: str) -> Optional[pd.DataFrame]:
        bars = self.get_stock_bars(code)
        if not bars:
            return None
        df = pd.DataFrame(bars)
        df["date"] = pd.to_datetime(df["date"])
        for col in ["open", "close", "high", "low", "volume"]:
            if col in df.columns:
                df[col] = pd.to_numeric(df[col], errors="coerce")
        if "turnover_rate" in df.columns:
            df["turnover_rate"] = pd.to_numeric(df["turnover_rate"], errors="coerce")
        df = df.sort_values("date").reset_index(drop=True)
        return df

    def stock_count(self) -> int:
        return self._conn.execute("SELECT COUNT(DISTINCT code) FROM daily_bars").fetchone()[0]


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


# ── HK-specific data sources ───────────────────────────────────

def _code_to_qq_hk(code: str) -> str:
    """港股代码转腾讯格式：00700 → hk00700"""
    c = str(code).zfill(5)
    return f"hk{c}"


def _fetch_bars_tencent_hk(symbol: str, days: int) -> Optional[List[Dict]]:
    """通过腾讯财经日线接口拉取港股前复权日线。"""
    try:
        qq_code = _code_to_qq_hk(symbol)
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
            logger.debug("[tencent-hk] no klines for %s (%s)", symbol, qq_code)
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
        logger.debug("[tencent-hk] fetch failed for %s: %s", symbol, exc)
        return None


def _fetch_bars_akshare_hk(symbol: str, days: int) -> Optional[List[Dict]]:
    """通过 AKShare 东财接口拉取港股日线（降级数据源）。"""
    try:
        import akshare as ak
        end_date = pd.Timestamp.today().strftime("%Y%m%d")
        start_date = (pd.Timestamp.today() - pd.Timedelta(days=days + 30)).strftime("%Y%m%d")
        df = ak.stock_hk_hist(
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
        logger.debug("[akshare-hk] fetch failed for %s: %s", symbol, exc)
        return None


def _fetch_daily_bars_hk(symbol: str, days: int = DAILY_LOOKBACK_DAYS) -> Optional[List[Dict]]:
    """港股日线拉取：腾讯优先，东财降级。"""
    bars = _fetch_bars_tencent_hk(symbol, days)
    if bars:
        return bars
    time.sleep(SINGLE_RETRY_DELAY_MS / 1000.0)
    bars = _fetch_bars_akshare_hk(symbol, days)
    return bars


def _fetch_hsi_60d_return() -> float:
    """通过腾讯财经获取恒生指数近 60 个交易日的收益率。"""
    # 恒生指数在腾讯的代码格式为 hkHSI
    hsi_codes = ["hkHSI", "hkHSI"]
    for code in hsi_codes:
        try:
            end_date = pd.Timestamp.today().strftime("%Y-%m-%d")
            start_date = (pd.Timestamp.today() - pd.Timedelta(days=180)).strftime("%Y-%m-%d")
            url = (
                f"https://web.ifzq.gtimg.cn/appstock/app/fqkline/get"
                f"?param={code},day,{start_date},{end_date},180,"
            )
            resp = requests.get(url, headers=_QQ_DAILY_HEADERS, timeout=15)
            resp.raise_for_status()
            data = resp.json()
            idx_data = data.get("data", {}).get(code, {})
            klines = idx_data.get("day") or idx_data.get("qfqday") or []
            if len(klines) < 2:
                continue
            closes = [float(k[2]) for k in klines if len(k) >= 3 and float(k[2]) > 0]
            if len(closes) < 2:
                continue
            lookback = min(60, len(closes))
            ret = (closes[-1] / closes[-lookback] - 1) * 100
            logger.info("恒生指数 60 日收益: %.2f%% (code=%s)", ret, code)
            return ret
        except Exception as exc:
            logger.debug("[hsi] failed for %s: %s", code, exc)

    logger.warning("恒生指数获取失败，基准收益默认 0.0")
    return 0.0


# ── Metrics computation ────────────────────────────────────────

def _percentile_rank(series: pd.Series) -> pd.Series:
    return series.rank(pct=True, na_option="keep") * 100


def _classify_a_share_board(code: str) -> str:
    code = str(code or "").strip()
    if code.startswith(("688", "689")):
        return "STAR"
    if code.startswith(("300", "301")):
        return "CHINEXT"
    if code.startswith(("600", "601", "603", "605", "000", "001", "002", "003")):
        return "MAIN"
    return "OTHER"


def _safe_momentum(return_pct: pd.Series, volatility: pd.Series, floor: float = 0.005) -> pd.Series:
    safe_vol = volatility.fillna(floor).clip(lower=floor)
    return return_pct.fillna(0) / safe_vol


# ── Float sanitization for JSON compliance ──────────────────────
# Python's standard json.dumps rejects NaN / Inf / -Inf (RFC 7159).
# Some stocks may have missing or degenerate daily-bar data that
# produce non-finite values even after fillna/clip.  This guard
# replaces them *before* serialization so the bulk-save callback
# never fails with "Out of range float values are not JSON compliant".
import math

def _finite(v: float, fallback: float = 0.0) -> float:
    """Return fallback when v is NaN or Inf; otherwise return v."""
    if math.isnan(v) or math.isinf(v):
        return fallback
    return v


def _sanitize_item(d: dict) -> dict:
    """Replace every non-finite float in dict with 0.0 (in-place)."""
    FLOAT_KEYS = {
        "opportunity", "risk", "trend", "flow", "revision",
        "liquidity", "volatility", "drawdown", "crowding",
        "avg_amount_5d", "ranking_score", "global_rank_score",
        "board_rank_score", "tradability_score", "risk_adjusted_momentum_60d",
        "close_price",
    }
    for k in FLOAT_KEYS:
        if k in d and isinstance(d[k], float):
            d[k] = _finite(d[k], 0.0)
    return d


def _latest_cached_close(cache: DailyBarCache, code: str) -> Tuple[float, str]:
    """Return the latest positive close and trade date from the daily-bar cache."""
    if cache is None or not code:
        return 0.0, ""
    bars = cache.get_stock_bars(code) or []
    for bar in reversed(bars):
        try:
            close_price = float(bar.get("close", 0) or 0)
        except (TypeError, ValueError):
            close_price = 0.0
        trade_date = str(bar.get("date", "") or "")
        if close_price > 0 and trade_date:
            return close_price, trade_date
    return 0.0, ""


def _compute_daily_metrics(daily_df: pd.DataFrame) -> Dict[str, float]:
    result: Dict[str, float] = {
        "std_20d": np.nan,
        "std_60d": np.nan,
        "max_drawdown_60d": np.nan,
        "turnover_20d_avg": np.nan,
        "cumulative_turnover_20d": np.nan,
        "change_pct_60d_calc": np.nan,
        "volume_ratio_calc": np.nan,
        "avg_amount_5d": np.nan,  # 近 5 日平均成交额（万元）
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

    if len(returns) >= 60:
        result["std_60d"] = float(returns.tail(60).std())
    elif len(returns) >= 20:
        result["std_60d"] = float(returns.std())

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

    # 近 5 日平均成交额（万元）：每日 volume × close 取均值
    # 用于流动性硬过滤 + Liquidity 子指标
    if vol_col and "close" in daily_df.columns:
        df_amount = daily_df[[vol_col, "close"]].dropna()
        if len(df_amount) >= 1:
            lookback = min(5, len(df_amount))
            amounts = (df_amount[vol_col].tail(lookback) * df_amount["close"].tail(lookback)) / 10000  # 转万元
            result["avg_amount_5d"] = float(amounts.mean())

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
    # ── 进度上报 URL（从 callback_url 派生）──
    _progress_url = _derive_progress_url(callback_url)

    # 上报：即将开始（让前端立刻知道任务已启动）
    _send_progress(_progress_url, "ASHARE", 0, 0, "running", error_msg=None,
                   message="正在初始化缓存数据库...")

    # ── 初始化缓存（带自动恢复）──
    logger.info("[quadrant] 初始化日线缓存...")
    try:
        cache = DailyBarCache()
    except Exception as exc:
        _send_progress(_progress_url, "ASHARE", 0, 0, "failed",
                       f"缓存初始化失败（磁盘 I/O 错误）：{exc}")
        raise

    _send_progress(_progress_url, "ASHARE", 0, 0, "running", error_msg=None,
                   message="正在拉取全市场快照...")

    # ── Step 1: 全市场快照 ──
    logger.info("[quadrant] Step 1: 拉取全市场快照...")
    try:
        snapshot_df = get_a_share_snapshot()
    except Exception as exc:
        _send_progress(_progress_url, "ASHARE", 0, 0, "failed",
                       f"拉取快照失败：{exc}")
        raise

    if snapshot_df is None or snapshot_df.empty:
        _send_progress(_progress_url, "ASHARE", 0, 0, "failed",
                       "全市场快照数据为空")
        raise RuntimeError("全市场快照数据为空")

    snapshot_df = snapshot_df[
        snapshot_df["code"].notna()
        & (snapshot_df["price"].notna())
        & (snapshot_df["price"] > 0)
    ].copy()
    all_codes = snapshot_df["code"].tolist()
    total_stocks = len(all_codes)
    logger.info("[quadrant] 有效股票: %d 只", total_stocks)

    # 上报：快照完成，更新真实总数
    _send_progress(_progress_url, "ASHARE", 0, total_stocks, "running",
                   message=f"快照加载完成（{total_stocks}只），准备拉取日线...")

    # ── Main computation: ensure failure path also writes log via callback ──
    result_items: list = []
    _compute_error: Optional[str] = None
    try:
        # ── Step 2: 决定全量 vs 增量 ──
        is_full = cache.needs_full_refresh(force_full=force_full)
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

        # 上报：开始拉日线
        _send_progress(_progress_url, "ASHARE", 0, total_stocks, "running")

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
                if done_count % _PROGRESS_REPORT_INTERVAL == 0:
                    logger.info("[quadrant] 日线进度: %d/%d (成功 %d)", done_count, total_stocks, success_count)
                    _send_progress(_progress_url, "ASHARE", done_count, total_stocks, "running")

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
        if not is_full:
            cached_count = cache.stock_count()
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
        merged["board"] = merged["code"].apply(_classify_a_share_board)

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

        # ── Revision (NaN-tolerant, field-missing-safe) ──
        # A股快照(scanner.py COLUMN_MAP)可能不包含 profit_growth_rate，
        # 此时用 PE 倒推质量信号（低 PE → 高 revision score）。
        if "profit_growth_rate" not in merged.columns or merged["profit_growth_rate"].isna().all():
            logger.info("[quadrant] 快照无利润增速字段，使用 PE 作为 Revision 替代信号")
            pe_for_rev = merged.get("pe", pd.Series(999.0, index=merged.index))
            merged["revision"] = _percentile_rank(-pe_for_rev.fillna(999)).fillna(50)
        else:
            merged["revision"] = _percentile_rank(merged["profit_growth_rate"]).fillna(50)

        # ── Liquidity (流动性：近 5 日均成交额 percentile rank) ──
        merged["liquidity_raw"] = _percentile_rank(merged["avg_amount_5d"]).fillna(50)

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
        l_score = merged["liquidity_raw"].fillna(50)

        std_60d = merged.get("std_60d", pd.Series(np.nan, index=merged.index))
        ram = _safe_momentum(merged["change_pct_60d"], std_60d)
        relative_ram = _safe_momentum(merged["change_pct_60d"] - bench_60d, std_60d)
        merged["risk_adjusted_momentum_60d"] = ram.replace([np.inf, -np.inf], np.nan).fillna(0)
        ram_rank = _percentile_rank(ram.replace([np.inf, -np.inf], np.nan)).fillna(50)
        relative_ram_rank = _percentile_rank(relative_ram.replace([np.inf, -np.inf], np.nan)).fillna(50)
        merged["ranking_trend"] = 0.5 * ram_rank + 0.5 * relative_ram_rank

        ranking_raw = (
            0.4 * merged["ranking_trend"].fillna(50)
            + 0.25 * merged["flow"].fillna(50)
            + 0.15 * merged["revision"].fillna(50)
            + 0.20 * merged["liquidity_raw"].fillna(50)
        )
        merged["global_rank_score"] = _percentile_rank(ranking_raw).fillna(50)
        merged["board_rank_score"] = 50.0
        for board, idx in merged.groupby("board").groups.items():
            merged.loc[idx, "board_rank_score"] = _percentile_rank(ranking_raw.loc[idx]).fillna(50)

        tradability_raw = np.log1p(merged["avg_amount_5d"].fillna(0).clip(lower=0))
        merged["tradability_score"] = _percentile_rank(tradability_raw).fillna(50)
        merged["ranking_score"] = (
            0.5 * merged["global_rank_score"].fillna(50)
            + 0.3 * merged["board_rank_score"].fillna(50)
            + 0.2 * merged["tradability_score"].fillna(50)
        )
        merged["ranking_score"] = merged["ranking_score"].fillna(50).clip(0, 100).round(2)

        # Opportunity = 0.4×Trend + 0.25×Flow + 0.15×Revision + 0.20×Liquidity
        merged["opportunity"] = 0.4 * t_score + 0.25 * f_score + 0.15 * r_score + 0.20 * l_score
        merged["risk"] = 0.4 * v_raw + 0.3 * d_raw + 0.3 * c_raw

        # ── Diagnostic: sub-score and final score stats ──
        for col in ["trend", "flow", "revision", "liquidity_raw",
                    "volatility_raw", "drawdown_raw", "crowding_raw", "opportunity", "risk"]:
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
        merged["global_rank_score"] = merged["global_rank_score"].fillna(50).clip(0, 100).round(2)
        merged["board_rank_score"] = merged["board_rank_score"].fillna(50).clip(0, 100).round(2)
        merged["tradability_score"] = merged["tradability_score"].fillna(50).clip(0, 100).round(2)
        merged["risk_adjusted_momentum_60d"] = merged["risk_adjusted_momentum_60d"].fillna(0).round(4)
        for col in ["trend", "flow", "revision", "liquidity_raw",
                    "volatility_raw", "drawdown_raw", "crowding_raw", "ranking_trend"]:
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
            # Determine exchange from A-share code prefix: 6xxxxx→SSE, 0/3xxxxx→SZSE
            _exchange = "SSE" if code.startswith("6") else "SZSE"
            close_price, price_trade_date = _latest_cached_close(cache, code)
            item = {
                "code": code,
                "name": name,
                "exchange": _exchange,
                "close_price": close_price,
                "price_trade_date": price_trade_date,
                "opportunity": float(row["opportunity"]),
                "risk": float(row["risk"]),
                "quadrant": row["quadrant"],
                "trend": float(row["trend"]),
                "flow": float(row["flow"]),
                "revision": float(row["revision"]),
                "liquidity": float(row["liquidity_raw"]),
                "volatility": float(row["volatility_raw"]),
                "drawdown": float(row["drawdown_raw"]),
                "crowding": float(row["crowding_raw"]),
                "avg_amount_5d": round(float(row.get("avg_amount_5d", 0) or 0), 2),
                "board": str(row.get("board", "")),
                "ranking_score": float(row.get("ranking_score", 0) or 0),
                "global_rank_score": float(row.get("global_rank_score", 0) or 0),
                "board_rank_score": float(row.get("board_rank_score", 0) or 0),
                "tradability_score": float(row.get("tradability_score", 0) or 0),
                "risk_adjusted_momentum_60d": float(row.get("risk_adjusted_momentum_60d", 0) or 0),
            }
            result_items.append(_sanitize_item(item))

        elapsed = time.time() - start_time
        mode_label = "全量" if is_full else "增量"
        logger.info("[quadrant] ✅ 计算完成 (%s): %d 只股票, 耗时 %.1f 秒", mode_label, len(result_items), elapsed)

        # ── Pre-flight JSON validation ──
        try:
            _json.dumps({"items": result_items[:1]}, ensure_ascii=False)
        except (ValueError, TypeError) as exc:
            logger.error("[quadrant] ❌ 结果数据包含非法 JSON 值 (NaN/Inf): %s — 将尝试自动修复", exc)
            for it in result_items:
                _sanitize_item(it)

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
            "exchange": "ASHARE",
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
        callback_ok, callback_err = _send_callback(callback_url, result_items, compute_report)

        # 上报终态：仅回调成功才报 success，失败时附带具体原因
        _send_progress(_progress_url, "ASHARE", total_stocks, total_stocks,
                       "success" if callback_ok else "failed",
                       None if callback_ok else f"数据未写入后端（回调失败）：{callback_err}")

        return result_items

    except Exception as exc:
        _compute_error = str(exc)
        logger.error("[quadrant] ❌ 计算异常: %s", _compute_error)
        import traceback
        traceback.print_exc()

        # ── 失败回调：确保计算历史记录这次失败的尝试 ──
        failed_report = {
            "computed_at": pd.Timestamp.now(tz="UTC").isoformat(),
            "mode": "全量" if is_full else "增量",
            "exchange": "ASHARE",
            "duration_seconds": round(time.time() - start_time, 1),
            "stock_count": 0,
            "status": "failed",
            "error": _compute_error,
        }
        _send_callback(callback_url, [], failed_report)

        # 进度终态：显示具体错误信息
        _send_progress(_progress_url, "ASHARE", total_stocks or 0,
                       total_stocks or 0, "failed",
                       f"计算异常：{_compute_error}")

        raise


def get_cached_scores() -> Optional[List[Dict[str, Any]]]:
    with _quadrant_cache_lock:
        if _quadrant_cache_data is not None and (time.time() - _quadrant_cache_ts) < _QUADRANT_CACHE_TTL:
            return _quadrant_cache_data
    return None


# ── HK Quadrant: dedicated cache & compute ─────────────────────
_hk_quadrant_cache_lock = threading.Lock()
_hk_quadrant_cache_data: Optional[List[Dict[str, Any]]] = None
_hk_quadrant_cache_ts: float = 0.0
_HK_QUADRANT_CACHE_TTL = 6 * 3600  # 6 hours

# HK daily bar cache — separate from A-share to avoid code collision
HK_CACHE_DB_PATH = os.path.join(_CACHE_DIR, "quadrant_cache_hk.db")


class HkDailyBarCache(DailyBarCache):
    """港股日线缓存，独立 SQLite 文件避免与 A 股代码冲突（5 位 vs 6 位）。"""

    def __init__(self):
        super().__init__(db_path=HK_CACHE_DB_PATH)


def compute_hk_quadrant_scores(
    callback_url: Optional[str] = None,
    force_full: bool = False,
) -> List[Dict[str, Any]]:
    """
    全市场港股四象限评分。

    数据源：
      - 快照：ak.stock_hk_spot_em()（东方财富港股实时行情）
      - 日线：腾讯财经优先，AKShare 东财降级
      - 基准：恒生指数（HSI）60 日收益
      - 基本面：复用 fundamentals.py 的 build_hk_payload

    Returns:
        List of dicts with code, name, opportunity, risk, quadrant, sub-scores
    """
    start_time = time.time()

    # Import here to avoid circular dependency at module level
    from screener.scanner import get_hk_snapshot

    # ── 进度上报 URL（从 callback_url 派生）──
    _hk_progress_url = _derive_progress_url(callback_url)

    # 上报：即将开始（缓存初始化前）
    _send_progress(_hk_progress_url, "HKEX", 0, 0, "running",
                   message="正在初始化港股缓存数据库...")

    # ── 初始化缓存（带自动恢复）──
    logger.info("[hk-quadrant] 初始化港股日线缓存...")
    try:
        cache = HkDailyBarCache()
    except Exception as exc:
        _send_progress(_hk_progress_url, "HKEX", 0, 0, "failed",
                       f"港股缓存初始化失败（磁盘 I/O 错误）：{exc}")
        raise

    _send_progress(_hk_progress_url, "HKEX", 0, 0, "running",
                   message="正在拉取港股全市场快照...")

    # ── Step 1: 港股全市场快照 ──
    logger.info("[hk-quadrant] Step 1: 拉取港股全市场快照...")
    try:
        snapshot_df = get_hk_snapshot()
    except Exception as exc:
        _send_progress(_hk_progress_url, "HKEX", 0, 0, "failed",
                       f"拉取港股快照失败：{exc}")
        raise

    if snapshot_df is None or snapshot_df.empty:
        _send_progress(_hk_progress_url, "HKEX", 0, 0, "failed",
                       "港股全市场快照数据为空")
        raise RuntimeError("港股全市场快照数据为空")

    # 过滤有效股票：有价格且 > 0
    snapshot_df = snapshot_df[
        snapshot_df["code"].notna()
        & (snapshot_df["price"].notna())
        & (snapshot_df["price"] > 0)
    ].copy()
    all_codes = snapshot_df["code"].tolist()
    total_stocks = len(all_codes)
    logger.info("[hk-quadrant] 有效股票: %d 只", total_stocks)

    # 上报：快照完成，更新真实总数
    _send_progress(_hk_progress_url, "HKEX", 0, total_stocks, "running",
                   message=f"港股快照加载完成（{total_stocks}只），准备拉取日线...")

    # ── Step 2: 决定全量 vs 增量 ──
    is_full = cache.needs_full_refresh(force_full=force_full)
    if is_full:
        fetch_days = DAILY_LOOKBACK_DAYS
        logger.info("[hk-quadrant] Step 2: 全量刷新模式 (拉取 %d 天日线)...", fetch_days)
    else:
        fetch_days = 3
        logger.info("[hk-quadrant] Step 2: 增量更新模式 (拉取 %d 天日线)...", fetch_days)

    # ── Step 3: 并发拉日线（港股用 _fetch_daily_bars_hk） ──
    logger.info("[hk-quadrant] 并发拉取 %d 只港股 (workers=%d, interval=%dms)...",
                total_stocks, MAX_WORKERS, REQUEST_INTERVAL_MS)

    # 上报：开始拉日线
    _send_progress(_hk_progress_url, "HKEX", 0, total_stocks, "running")

    success_count = 0
    failed_codes: List[str] = []

    def fetch_with_interval_hk(code: str) -> Tuple[str, Optional[List[Dict]]]:
        bars = _fetch_daily_bars_hk(code, fetch_days)
        if REQUEST_INTERVAL_MS > 0:
            time.sleep(REQUEST_INTERVAL_MS / 1000.0)
        return code, bars

    with ThreadPoolExecutor(max_workers=MAX_WORKERS) as executor:
        futures = {executor.submit(fetch_with_interval_hk, code): code for code in all_codes}
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
            if done_count % _PROGRESS_REPORT_INTERVAL == 0:
                logger.info("[hk-quadrant] 日线进度: %d/%d (成功 %d)", done_count, total_stocks, success_count)
                _send_progress(_hk_progress_url, "HKEX", done_count, total_stocks, "running")

    fetch_ratio = success_count / total_stocks if total_stocks > 0 else 0
    logger.info("[hk-quadrant] 日线完成: 成功 %d / 总 %d (%.1f%%)",
                success_count, total_stocks, fetch_ratio * 100)

    if is_full and fetch_ratio < MIN_SUCCESS_RATIO:
        raise RuntimeError(
            f"港股日线拉取成功率过低: {success_count}/{total_stocks} ({fetch_ratio:.1%})，"
            f"阈值 {MIN_SUCCESS_RATIO:.0%}"
        )

    if not is_full:
        cached_count = cache.stock_count()
        cached_ratio = cached_count / total_stocks if total_stocks > 0 else 0
        logger.info("[hk-quadrant] 缓存覆盖: %d / %d (%.1f%%)", cached_count, total_stocks, cached_ratio * 100)

    # Update cache metadata
    if is_full:
        cache.mark_full_refresh()
    else:
        cache.mark_incremental()
    cache.save()

    # ── Step 4: 恒生指数 60 日收益 ──
    logger.info("[hk-quadrant] Step 4: 拉取恒生指数 60 日收益...")
    bench_60d = _fetch_hsi_60d_return()
    logger.info("[hk-quadrant] 恒生 60 日收益: %.2f%%", bench_60d)

    # ── Step 4.5: 将快照 turnover_rate 注入日线缓存 ──
    injected_count = 0
    for _, row in snapshot_df.iterrows():
        code = row.get("code")
        tr = row.get("turnover_rate")
        if code and pd.notna(tr) and tr > 0:
            bars = cache.get_stock_bars(code)
            if bars and len(bars) > 0:
                bars[-1]["turnover_rate"] = float(tr)
                injected_count += 1
    logger.info("[hk-quadrant] 快照换手率注入日线缓存: %d 只股票", injected_count)

    # ── Step 5: 计算子指标（复用 A 股的计算逻辑） ──
    logger.info("[hk-quadrant] Step 5: 计算子指标...")

    daily_metrics_rows = []
    for code in all_codes:
        daily_df = cache.bars_to_dataframe(code)
        metrics = _compute_daily_metrics(daily_df)
        metrics["code"] = code
        daily_metrics_rows.append(metrics)

    daily_metrics_df = pd.DataFrame(daily_metrics_rows)
    merged = snapshot_df.merge(daily_metrics_df, on="code", how="left")

    # Backfill missing fields from daily-bar calculations
    if "change_pct_60d" in merged.columns:
        merged["change_pct_60d"] = merged["change_pct_60d"].fillna(merged["change_pct_60d_calc"])
    else:
        merged["change_pct_60d"] = merged["change_pct_60d_calc"]
    if "volume_ratio" in merged.columns:
        merged["volume_ratio"] = merged["volume_ratio"].fillna(merged["volume_ratio_calc"])
    else:
        merged["volume_ratio"] = merged["volume_ratio_calc"]

    # ── Trend (NaN-tolerant) ──
    c60d = merged.get("change_pct_60d", pd.Series(np.nan, index=merged.index))
    change_60d_rank = _percentile_rank(c60d).fillna(50)
    excess_return = c60d.fillna(0) - bench_60d
    excess_rank = _percentile_rank(excess_return).fillna(50)
    merged["trend"] = 0.5 * change_60d_rank + 0.5 * excess_rank

    # ── Flow (NaN-tolerant, 列可能因数据源不同而缺失) ──
    volume_ratio_rank = _percentile_rank(merged["volume_ratio"]).fillna(50) if "volume_ratio" in merged.columns else pd.Series(50.0, index=merged.index)
    turnover_rate_rank = _percentile_rank(merged["turnover_rate"]).fillna(50) if "turnover_rate" in merged.columns else pd.Series(50.0, index=merged.index)
    if "turnover" in merged.columns and "turnover_20d_avg" in merged.columns:
        turnover_ratio = merged["turnover"] / merged["turnover_20d_avg"].replace(0, np.nan)
        turnover_ratio_rank = _percentile_rank(turnover_ratio).fillna(50)
    else:
        turnover_ratio_rank = pd.Series(50.0, index=merged.index)
    merged["flow"] = 0.4 * volume_ratio_rank + 0.3 * turnover_rate_rank + 0.3 * turnover_ratio_rank

    # ── Revision: 港股使用快照 PE/PB 作为基本面参考，
    #   利润增速从 fundamentals.py 补充（如果可用），否则用 PE 倒推质量信号
    #   快照中可能没有 profit_growth_rate，尝试从基本面补充
    if "profit_growth_rate" not in merged.columns or merged["profit_growth_rate"].isna().all():
        logger.info("[hk-quadrant] 快照无利润增速字段，使用 PE 作为 Revision 替代信号")
        pe_for_revision = merged.get("pe", pd.Series(999.0, index=merged.index))
        # 低 PE → 高 revision score（估值低=机会）
        merged["revision"] = _percentile_rank(-pe_for_revision.fillna(999)).fillna(50)
    else:
        merged["revision"] = _percentile_rank(merged["profit_growth_rate"]).fillna(50)

    # ── Volatility ──
    merged["volatility_raw"] = _percentile_rank(merged.get("std_20d", pd.Series(dtype=float))).fillna(50)

    # ── Drawdown ──
    merged["drawdown_raw"] = _percentile_rank(merged.get("max_drawdown_60d", pd.Series(dtype=float))).fillna(50)

    # ── Crowding ──
    pe_col = merged.get("pe", pd.Series(np.nan, index=merged.index))
    pe_rank = _percentile_rank(pe_col).fillna(50)
    cum_tr_col = merged.get("cumulative_turnover_20d", pd.Series(dtype=float))
    cum_turnover_rank = _percentile_rank(cum_tr_col).fillna(50)
    has_cum_turnover = cum_tr_col.notna()
    merged["crowding_raw"] = pd.Series(np.where(
        has_cum_turnover,
        0.5 * pe_rank + 0.5 * cum_turnover_rank,
        pe_rank,  # fallback: crowding = PE rank only
    ), index=merged.index)
    logger.info("[hk-quadrant] Crowding: %d 只用 PE+换手率, %d 只仅用 PE",
                int(has_cum_turnover.sum()), int((~has_cum_turnover).sum()))

    # ── Liquidity (流动性：近 5 日均成交额 percentile rank) ──
    merged["liquidity_raw"] = _percentile_rank(merged.get("avg_amount_5d", pd.Series(dtype=float))).fillna(50)

    # ── Final scores ──
    v_raw = merged["volatility_raw"].fillna(50)
    d_raw = merged["drawdown_raw"].fillna(50)
    c_raw = merged["crowding_raw"].fillna(50)
    t_score = merged["trend"].fillna(50)
    f_score = merged["flow"].fillna(50)
    r_score = merged["revision"].fillna(50)
    l_score = merged["liquidity_raw"].fillna(50)

    # Opportunity = 0.4×Trend + 0.25×Flow + 0.15×Revision + 0.20×Liquidity
    merged["opportunity"] = 0.4 * t_score + 0.25 * f_score + 0.15 * r_score + 0.20 * l_score
    merged["risk"] = 0.4 * v_raw + 0.3 * d_raw + 0.3 * c_raw

    # Re-normalize
    merged["opportunity"] = _percentile_rank(merged["opportunity"].fillna(50))
    merged["risk"] = _percentile_rank(merged["risk"].fillna(50))

    merged["opportunity"] = merged["opportunity"].fillna(50).clip(0, 100).round(2)
    merged["risk"] = merged["risk"].fillna(50).clip(0, 100).round(2)
    for col in ["trend", "flow", "revision", "liquidity_raw",
                "volatility_raw", "drawdown_raw", "crowding_raw"]:
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
        close_price, price_trade_date = _latest_cached_close(cache, code)
        item = {
            "code": code,
            "name": name,
            "exchange": "HKEX",
            "close_price": close_price,
            "price_trade_date": price_trade_date,
            "opportunity": float(row["opportunity"]),
            "risk": float(row["risk"]),
            "quadrant": row["quadrant"],
            "trend": float(row["trend"]),
            "flow": float(row["flow"]),
            "revision": float(row["revision"]),
            "liquidity": float(row["liquidity_raw"]),
            "volatility": float(row["volatility_raw"]),
            "drawdown": float(row["drawdown_raw"]),
            "crowding": float(row["crowding_raw"]),
            "avg_amount_5d": round(float(row.get("avg_amount_5d", 0) or 0), 2),
        }
        result_items.append(_sanitize_item(item))

    elapsed = time.time() - start_time
    mode_label = "全量" if is_full else "增量"
    logger.info("[hk-quadrant] ✅ 计算完成 (%s): %d 只港股, 耗时 %.1f 秒", mode_label, len(result_items), elapsed)

    # ── Pre-flight JSON validation (same guard as A-share) ──
    try:
        _json.dumps({"items": result_items[:1]}, ensure_ascii=False)
    except (ValueError, TypeError) as exc:
        logger.error("[hk-quadrant] ❌ 结果数据包含非法 JSON 值 (NaN/Inf): %s — 将尝试自动修复", exc)
        for it in result_items:
            _sanitize_item(it)

    # Build report
    quadrant_counts = {}
    for item in result_items:
        q = item["quadrant"]
        quadrant_counts[q] = quadrant_counts.get(q, 0) + 1

    compute_report = {
        "computed_at": pd.Timestamp.now(tz="UTC").isoformat(),
        "mode": mode_label,
        "exchange": "HKEX",
        "duration_seconds": round(elapsed, 1),
        "stock_count": len(result_items),
        "daily_bars": {
            "success": success_count,
            "failed": len(failed_codes),
            "total": total_stocks,
        },
        "quadrant_counts": quadrant_counts,
        "status": "success",
        "error": "",
    }
    logger.info("[hk-quadrant] 计算报告: %s", _json.dumps(compute_report, ensure_ascii=False))

    # Update in-memory cache
    with _hk_quadrant_cache_lock:
        global _hk_quadrant_cache_data, _hk_quadrant_cache_ts
        _hk_quadrant_cache_data = result_items
        _hk_quadrant_cache_ts = time.time()

    # Callback to Go backend (include report)
    callback_ok, callback_err = _send_callback(callback_url, result_items, compute_report)

    # 上报终态：仅回调成功才报 success，失败时附带具体原因
    _send_progress(_hk_progress_url, "HKEX", total_stocks, total_stocks,
                   "success" if callback_ok else "failed",
                   None if callback_ok else f"数据未写入后端（回调失败）：{callback_err}")

    return result_items


def get_cached_hk_scores() -> Optional[List[Dict[str, Any]]]:
    with _hk_quadrant_cache_lock:
        if _hk_quadrant_cache_data is not None and (time.time() - _hk_quadrant_cache_ts) < _HK_QUADRANT_CACHE_TTL:
            return _hk_quadrant_cache_data
    return None


def _send_callback(callback_url: str, items: List[Dict[str, Any]], report: Optional[Dict] = None) -> tuple:
    """向 Go 后端 POST bulk-save 数据。返回 (ok: bool, error_msg: str)。
    
    返回值：
      (True, "")          — 回调成功
      (False, "具体原因")  — 回调失败（含 HTTP 状态码/异常信息）
    """
    try:
        payload = {
            "items": items,
            "computed_at": pd.Timestamp.now(tz="UTC").isoformat(),
        }
        if report:
            payload["report"] = report
        resp = requests.post(
            callback_url, json=payload, timeout=120,
            headers={"Content-Type": "application/json"},
        )
        if resp.status_code < 200 or resp.status_code >= 300:
            body_preview = resp.text[:300] if resp.text else "(empty)"
            err_msg = f"HTTP {resp.status_code}: {body_preview}"
            logger.warning("[quadrant] ❌ 回调失败: %s", err_msg)
            return False, err_msg
        else:
            logger.info("[quadrant] ✅ 回调成功: HTTP %d, 写入 %d 条", resp.status_code, len(items))
            return True, ""
    except requests.exceptions.Timeout as exc:
        err_msg = f"请求超时(120s): {exc}"
        logger.error("[quadrant] ❌ 回调超时: %s", exc)
        return False, err_msg
    except requests.exceptions.ConnectionError as exc:
        err_msg = f"连接失败: {exc}"
        logger.error("[quadrant] ❌ 回调连接错误: %s", exc)
        return False, err_msg
    except Exception as exc:
        err_msg = f"异常: {exc}"
        logger.error("[quadrant] ❌ 回调异常: %s (数据未写入后端)", exc)
        return False, err_msg


# ── Progress reporting to Go backend ──────────────────────────────

_PROGRESS_REPORT_INTERVAL = 100  # 每 N 只股票上报一次进度


def _derive_progress_url(callback_url: Optional[str]) -> Optional[str]:
    """从 bulk-save callback URL 推导出 progress URL。
    
    例如: http://backend:8080/api/quadrant/bulk-save
       → http://backend:8080/api/quadrant/progress
    """
    if not callback_url:
        return None
    return callback_url.rstrip("/").removesuffix("/bulk-save") + "/progress"


def _send_progress(progress_url: Optional[str], exchange: str,
                   current: int, total: int, status: str,
                   error_msg: Optional[str] = None,
                   message: Optional[str] = None):
    """向 Go 后端上报计算进度（fire-and-forget，失败不阻塞主流程）。
    
    Args:
        progress_url: POST 目标 URL（由 callback_url 派生）
        exchange:     "ASHARE" 或 "HKEX"
        current:      已完成数量
        total:        总数量（可能为 0 表示未知）
        status:       "running" / "success" / "failed"
        error_msg:    失败时的具体原因（仅 status="failed" 时有意义）
        message:      阶段描述信息，如「正在拉取全市场快照...」
    """
    if not progress_url:
        return
    try:
        payload = {
            "exchange": exchange,
            "current": current,
            "total": total,
            "status": status,
        }
        if error_msg:
            payload["error_msg"] = error_msg
        if message:
            payload["message"] = message
        resp = requests.post(
            progress_url, json=payload, timeout=5,
            headers={"Content-Type": "application/json"},
        )
        if resp.status_code < 200 or resp.status_code >= 300:
            logger.debug("[progress] 上报异常: HTTP %d (%s)", resp.status_code, exchange)
    except Exception:
        pass  # 静默：进度上报不可用不应影响主计算
