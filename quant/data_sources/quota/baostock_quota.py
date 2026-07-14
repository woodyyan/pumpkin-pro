"""
全局 Baostock 配额守卫 — 服务级单例。

设计依据：
- baostock 的 50000 次/日配额是单 IP 限制，不分账号。
- 仓库内有多处裸调用 baostock 的代码路径（四象限、Factor Lab 行业回填、一次性脚本），
  它们共享同一份 IP 配额预算。
- 本模块提供全局唯一的配额账本，所有 baostock 调用方必须经过 try_acquire() 检查。

特性：
1. 进程内单例 + SQLite 落盘跨进程共享。
2. caller 维度归因统计（不按 caller 分割配额，只做统计标注）。
3. 超额时标记 blacklist，当天不再放行。
4. 读写原子性通过 SQLite 事务保证。
"""

from __future__ import annotations

import logging
import os
import sqlite3
import threading
from datetime import datetime, timezone
from typing import Dict, Optional

logger = logging.getLogger(__name__)

# 配额默认值
DEFAULT_DAILY_QUOTA = 50000
# 熔断阈值：用量达到此比例后标记为黑名单，当天不再放行
BLACKLIST_THRESHOLD_RATIO = 0.90  # 45000 次

# 落盘路径
_CACHE_DIR = os.path.join(
    os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__)))),
    "data",
    "cache",
)
DEFAULT_QUOTA_DB_PATH = os.path.join(_CACHE_DIR, "baostock_quota.db")


class GlobalBaostockQuotaGuard:
    """
    全局 baostock 配额守卫（服务级单例）。

    使用 SQLite 落盘，支持跨进程共享同一份配额账本。
    进程内通过 threading.Lock 保证线程安全，跨进程通过 SQLite 事务保证原子性。
    """

    _instance: Optional["GlobalBaostockQuotaGuard"] = None
    _instance_lock = threading.Lock()

    def __new__(cls, *args, **kwargs):
        if cls._instance is None:
            with cls._instance_lock:
                if cls._instance is None:
                    cls._instance = super().__new__(cls)
        return cls._instance

    def __init__(self, db_path: Optional[str] = None, daily_quota: int = DEFAULT_DAILY_QUOTA):
        # __init__ 可能被多次调用（单例模式），只初始化一次
        if hasattr(self, "_initialized"):
            return
        self._initialized = True

        self._db_path = db_path or DEFAULT_QUOTA_DB_PATH
        self._daily_quota = daily_quota
        self._lock = threading.Lock()
        self._blacklist_threshold = int(daily_quota * BLACKLIST_THRESHOLD_RATIO)

        # 确保目录和表存在
        os.makedirs(os.path.dirname(self._db_path), exist_ok=True)
        self._init_db()

        logger.info(
            "[baostock_quota] 全局配额守卫已初始化 (db=%s, quota=%d, blacklist_threshold=%d)",
            self._db_path,
            self._daily_quota,
            self._blacklist_threshold,
        )

    def _init_db(self) -> None:
        """初始化 SQLite 表结构。"""
        conn = sqlite3.connect(self._db_path, timeout=10)
        try:
            conn.execute("PRAGMA journal_mode=WAL")
            conn.execute("PRAGMA busy_timeout=5000")
            # 主表：每日配额汇总
            conn.execute("""
                CREATE TABLE IF NOT EXISTS baostock_quota (
                    date TEXT PRIMARY KEY,
                    used_count INTEGER NOT NULL DEFAULT 0,
                    threshold INTEGER NOT NULL,
                    blacklisted INTEGER NOT NULL DEFAULT 0,
                    blacklisted_at TEXT,
                    blacklisted_reason TEXT
                )
            """)
            # 明细表：每次调用的归因日志（可选，用于 by_caller 统计和审计）
            conn.execute("""
                CREATE TABLE IF NOT EXISTS baostock_quota_usage_log (
                    id INTEGER PRIMARY KEY AUTOINCREMENT,
                    date TEXT NOT NULL,
                    caller TEXT NOT NULL DEFAULT 'unknown',
                    ts TEXT NOT NULL,
                    cost INTEGER NOT NULL DEFAULT 1
                )
            """)
            conn.execute(
                "CREATE INDEX IF NOT EXISTS idx_quota_log_date_caller "
                "ON baostock_quota_usage_log(date, caller)"
            )
            conn.commit()
        finally:
            conn.close()

    def _today_str(self) -> str:
        """返回 UTC 日期字符串（baostock 限按自然日重置）。"""
        return datetime.now(timezone.utc).strftime("%Y-%m-%d")

    def try_acquire(self, cost: int = 1, caller: str = "unknown") -> bool:
        """
        尝试申请配额。

        Args:
            cost:  本次申请的配额消耗（默认 1）。
            caller: 调用方标识（如 "quadrant"/"factor_lab_growth_backfill"），
                    仅用于统计归因，不影响配额扣减逻辑。

        Returns:
            True 如果配额充足且未黑名单；False 如果已黑名单或余量不足。
        """
        today = self._today_str()

        with self._lock:
            conn = sqlite3.connect(self._db_path, timeout=10)
            try:
                conn.execute("PRAGMA journal_mode=WAL")
                conn.execute("PRAGMA busy_timeout=5000")
                conn.execute("BEGIN IMMEDIATE")

                # 读取或创建当日记录
                row = conn.execute(
                    "SELECT used_count, blacklisted FROM baostock_quota WHERE date = ?",
                    (today,),
                ).fetchone()

                if row is None:
                    conn.execute(
                        "INSERT INTO baostock_quota (date, used_count, threshold, blacklisted) "
                        "VALUES (?, 0, ?, 0)",
                        (today, self._daily_quota),
                    )
                    used_count = 0
                    blacklisted = 0
                else:
                    used_count, blacklisted = row

                # 黑名单检查
                if blacklisted:
                    conn.rollback()
                    logger.warning(
                        "[baostock_quota] 配额已黑名单（今日已用 %d/%d），拒绝 caller=%s",
                        used_count, self._daily_quota, caller,
                    )
                    return False

                # 余量检查
                remaining = self._daily_quota - used_count
                if remaining < cost:
                    conn.rollback()
                    logger.warning(
                        "[baostock_quota] 配额不足（已用 %d/%d，剩余 %d，需 %d），拒绝 caller=%s",
                        used_count, self._daily_quota, remaining, cost, caller,
                    )
                    return False

                # 扣减配额
                new_count = used_count + cost
                conn.execute(
                    "UPDATE baostock_quota SET used_count = ? WHERE date = ?",
                    (new_count, today),
                )

                # 写入明细日志
                conn.execute(
                    "INSERT INTO baostock_quota_usage_log (date, caller, ts, cost) "
                    "VALUES (?, ?, ?, ?)",
                    (today, caller, datetime.now(timezone.utc).isoformat(), cost),
                )

                # 黑名单触发检查
                if new_count >= self._blacklist_threshold and not blacklisted:
                    conn.execute(
                        "UPDATE baostock_quota SET blacklisted = 1, "
                        "blacklisted_at = ?, blacklisted_reason = ? "
                        "WHERE date = ?",
                        (
                            datetime.now(timezone.utc).isoformat(),
                            f"used {new_count}/{self._daily_quota} reached threshold",
                            today,
                        ),
                    )
                    logger.warning(
                        "[baostock_quota] 配额达到熔断阈值 %d/%d（%.0f%%），"
                        "当天剩余请求将被拒绝",
                        new_count, self._daily_quota,
                        BLACKLIST_THRESHOLD_RATIO * 100,
                    )

                conn.commit()

                logger.debug(
                    "[baostock_quota] 配额扣减成功: %d → %d/%d (caller=%s)",
                    used_count, new_count, self._daily_quota, caller,
                )
                return True
            except Exception as exc:
                conn.rollback()
                logger.error("[baostock_quota] 配额检查异常: %s", exc)
                # 异常时放行（宁可多请求也不要因为配额守卫 bug 阻塞业务）
                return True
            finally:
                conn.close()

    def is_blacklisted(self) -> bool:
        """检查今天是否已被标记为黑名单。"""
        today = self._today_str()
        with self._lock:
            conn = sqlite3.connect(self._db_path, timeout=10)
            try:
                row = conn.execute(
                    "SELECT blacklisted FROM baostock_quota WHERE date = ?",
                    (today,),
                ).fetchone()
                return bool(row and row[0])
            finally:
                conn.close()

    def mark_blacklisted(self, reason: str = "manual") -> None:
        """手动标记当天为黑名单。"""
        today = self._today_str()
        with self._lock:
            conn = sqlite3.connect(self._db_path, timeout=10)
            try:
                conn.execute("BEGIN IMMEDIATE")
                conn.execute(
                    "INSERT OR IGNORE INTO baostock_quota (date, used_count, threshold, blacklisted) "
                    "VALUES (?, 0, ?, 0)",
                    (today, self._daily_quota),
                )
                conn.execute(
                    "UPDATE baostock_quota SET blacklisted = 1, "
                    "blacklisted_at = ?, blacklisted_reason = ? "
                    "WHERE date = ?",
                    (datetime.now(timezone.utc).isoformat(), reason, today),
                )
                conn.commit()
                logger.warning("[baostock_quota] 手动标记黑名单: %s", reason)
            except Exception as exc:
                conn.rollback()
                logger.error("[baostock_quota] 标记黑名单失败: %s", exc)
            finally:
                conn.close()

    def snapshot(self) -> Dict:
        """
        返回当前配额状态快照，供 Admin 健康面板展示。

        Returns:
            {
                "date": "2026-07-14",
                "used_count": 3500,
                "daily_quota": 50000,
                "remaining": 46500,
                "blacklisted": false,
                "blacklisted_at": null,
                "blacklisted_reason": null,
                "by_caller": {"quadrant": 3400, "factor_lab_growth_backfill": 100},
                "usage_ratio": 0.07,
            }
        """
        today = self._today_str()
        with self._lock:
            conn = sqlite3.connect(self._db_path, timeout=10)
            try:
                row = conn.execute(
                    "SELECT used_count, threshold, blacklisted, blacklisted_at, blacklisted_reason "
                    "FROM baostock_quota WHERE date = ?",
                    (today,),
                ).fetchone()

                if row is None:
                    return {
                        "date": today,
                        "used_count": 0,
                        "daily_quota": self._daily_quota,
                        "remaining": self._daily_quota,
                        "blacklisted": False,
                        "blacklisted_at": None,
                        "blacklisted_reason": None,
                        "by_caller": {},
                        "usage_ratio": 0.0,
                    }

                used_count, threshold, blacklisted, bl_at, bl_reason = row

                # 按 caller 汇总今日用量
                caller_rows = conn.execute(
                    "SELECT caller, SUM(cost) as total "
                    "FROM baostock_quota_usage_log "
                    "WHERE date = ? "
                    "GROUP BY caller",
                    (today,),
                ).fetchall()
                by_caller = {r[0]: r[1] for r in caller_rows}

                remaining = self._daily_quota - used_count
                usage_ratio = used_count / self._daily_quota if self._daily_quota > 0 else 0.0

                return {
                    "date": today,
                    "used_count": used_count,
                    "daily_quota": self._daily_quota,
                    "remaining": remaining,
                    "blacklisted": bool(blacklisted),
                    "blacklisted_at": bl_at,
                    "blacklisted_reason": bl_reason,
                    "by_caller": by_caller,
                    "usage_ratio": round(usage_ratio, 4),
                }
            finally:
                conn.close()


# ── 模块级单例获取入口 ──────────────────────────────────────────

_global_guard: Optional[GlobalBaostockQuotaGuard] = None
_global_guard_lock = threading.Lock()


def get_global_quota_guard() -> GlobalBaostockQuotaGuard:
    """获取全局配额守卫单例。"""
    global _global_guard
    if _global_guard is None:
        with _global_guard_lock:
            if _global_guard is None:
                _global_guard = GlobalBaostockQuotaGuard()
    return _global_guard
