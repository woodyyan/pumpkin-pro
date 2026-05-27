#!/usr/bin/env python3
"""Factor Lab Phase 0 daily incremental updater.

This command is intentionally separate from the one-off backfill command. It is
for production daily jobs: refresh the smallest useful data window, keep writes
idempotent, and continue non-critical modes when possible.
"""

from __future__ import annotations

import argparse
import json
import os
import sqlite3
import subprocess
import sys
import tempfile
from dataclasses import dataclass
from datetime import datetime, timedelta
from pathlib import Path
from typing import Optional

SCRIPT_DIR = Path(__file__).resolve().parent
if str(SCRIPT_DIR) not in sys.path:
    sys.path.insert(0, str(SCRIPT_DIR))

from backfill_factor_lab_phase0 import connect_db, ensure_schema, log_step, resolve_db_path  # noqa: E402

BACKFILL_SCRIPT = SCRIPT_DIR / "backfill_factor_lab_phase0.py"
DEFAULT_LOOKBACK_DAYS = 500
DEFAULT_BUFFER_DAYS = 7
DEFAULT_FINANCIAL_REPORT_LIMIT = 2
DEFAULT_DIVIDEND_REPORT_LIMIT = 2
DEFAULT_STEP_TIMEOUT_SECONDS = 1800
DEFAULT_MODES = ("securities", "daily-bars", "index-bars", "financials", "dividends")
DEFAULT_CRITICAL_MODES = {"securities", "daily-bars", "index-bars"}


@dataclass
class ModeResult:
    mode: str
    status: str
    return_code: int
    duration_seconds: float
    error: str = ""


def parse_args(argv: list[str]) -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Factor Lab Phase 0 daily incremental updater")
    parser.add_argument("--db", default="", help="pumpkin.db 路径；默认自动查找 data/pumpkin.db")
    parser.add_argument("--write", action="store_true", help="实际写入数据库；默认 dry-run")
    parser.add_argument("--modes", default=",".join(DEFAULT_MODES), help="逗号分隔的模式列表")
    parser.add_argument("--scope", choices=["incremental", "repair_missing_dividend_yield", "repair_missing_fcfm_inputs"], default="incremental", help="增量或字段修复范围")
    parser.add_argument("--critical-modes", default=",".join(sorted(DEFAULT_CRITICAL_MODES)), help="失败后必须返回 failed 的关键模式")
    parser.add_argument("--lookback-days", type=int, default=DEFAULT_LOOKBACK_DAYS, help="无本地日期时的兜底回看自然日")
    parser.add_argument("--buffer-days", type=int, default=DEFAULT_BUFFER_DAYS, help="从本地最新日期向前回退的增量缓冲天数")
    parser.add_argument("--financial-report-limit", type=int, default=DEFAULT_FINANCIAL_REPORT_LIMIT, help="财务增量扫描最近报告期数量")
    parser.add_argument("--dividend-report-limit", type=int, default=DEFAULT_DIVIDEND_REPORT_LIMIT, help="分红增量扫描最近报告期数量")
    parser.add_argument("--step-timeout-seconds", type=int, default=DEFAULT_STEP_TIMEOUT_SECONDS, help="每个子任务超时时间")
    parser.add_argument("--progress-interval", type=int, default=500, help="普通子任务每处理多少项输出一次进度")
    parser.add_argument("--item-progress-interval", type=int, default=1, help="逐股票子任务每处理多少项输出一次进度")
    parser.add_argument("--daily-bars-source", choices=["auto", "akshare", "eastmoney", "tencent"], default="tencent", help="日线增量数据源，默认腾讯以减少逐股慢失败")
    parser.add_argument("--financials-source", choices=["auto", "akshare", "eastmoney", "tencent"], default="auto", help="财务增量数据源")
    parser.add_argument("--dividends-source", choices=["auto", "akshare", "eastmoney", "tencent"], default="auto", help="分红增量数据源")
    parser.add_argument("--sleep", type=float, default=0.05, help="单股票请求间隔秒数，避免外部源限流")
    parser.add_argument("--snapshot-date", default="", help="市场快照交易日 YYYY-MM-DD；默认今天")
    parser.add_argument("--python-bin", default=sys.executable, help="用于调用 backfill 脚本的 Python 解释器")
    return parser.parse_args(argv)


def split_csv(value: str) -> list[str]:
    return [item.strip() for item in str(value or "").split(",") if item.strip()]


def parse_date(value: str) -> Optional[datetime]:
    text = str(value or "").strip()
    if not text:
        return None
    for fmt in ("%Y-%m-%d", "%Y%m%d"):
        try:
            return datetime.strptime(text[:10] if fmt == "%Y-%m-%d" else text[:8], fmt)
        except ValueError:
            continue
    return None


def yyyymmdd(value: datetime) -> str:
    return value.strftime("%Y%m%d")


def buffered_start_date(latest_date: str, buffer_days: int) -> str:
    parsed = parse_date(latest_date)
    if not parsed:
        return ""
    return yyyymmdd(parsed - timedelta(days=max(buffer_days, 0)))


def latest_date(conn: sqlite3.Connection, table: str, column: str, where: str = "") -> str:
    query = f"SELECT MAX({column}) FROM {table}"
    if where:
        query += f" WHERE {where}"
    try:
        row = conn.execute(query).fetchone()
    except sqlite3.Error:
        return ""
    return str(row[0] or "") if row else ""


def active_security_codes(conn: sqlite3.Connection) -> list[str]:
    try:
        rows = conn.execute(
            """
            SELECT code FROM factor_securities
            WHERE is_active = 1 AND is_st = 0 AND board IN ('MAIN', 'CHINEXT')
            ORDER BY code ASC
            """
        ).fetchall()
    except sqlite3.Error:
        return []
    return [str(row[0]) for row in rows if row and row[0]]


def codes_missing_daily_target(conn: sqlite3.Connection, target_date: str) -> list[str]:
    if not target_date:
        return active_security_codes(conn)
    try:
        rows = conn.execute(
            """
            SELECT s.code
            FROM factor_securities s
            LEFT JOIN (
              SELECT code, MAX(trade_date) AS max_trade_date
              FROM factor_daily_bars
              GROUP BY code
            ) b ON b.code = s.code
            WHERE s.is_active = 1 AND s.is_st = 0 AND s.board IN ('MAIN', 'CHINEXT')
              AND COALESCE(b.max_trade_date, '') < ?
            ORDER BY s.code ASC
            """,
            (target_date,),
        ).fetchall()
    except sqlite3.Error:
        return active_security_codes(conn)
    return [str(row[0]) for row in rows]


def latest_report_candidates(limit: int) -> list[str]:
    today = datetime.today()
    year = today.year
    candidates = [f"{year}-09-30", f"{year}-06-30", f"{year}-03-31", f"{year - 1}-12-31", f"{year - 1}-09-30", f"{year - 1}-06-30"]
    parsed_today = today.strftime("%Y-%m-%d")
    return [item for item in candidates if item <= parsed_today][:max(limit, 1)]


def codes_missing_fcfm_inputs(conn: sqlite3.Connection) -> list[str]:
    try:
        rows = conn.execute(
            """
            SELECT s.code
            FROM factor_securities s
            LEFT JOIN factor_financial_metrics f ON f.code = s.code AND f.report_period = (
              SELECT MAX(f2.report_period)
              FROM factor_financial_metrics f2
              WHERE f2.code = s.code
            )
            WHERE s.is_active = 1 AND s.is_st = 0 AND s.board IN ('MAIN', 'CHINEXT')
              AND (
                f.code IS NULL
                OR f.revenue IS NULL
                OR f.operating_cash_flow IS NULL
                OR f.capex IS NULL
              )
            ORDER BY s.code ASC
            """
        ).fetchall()
    except sqlite3.Error:
        return active_security_codes(conn)
    return [str(row[0]) for row in rows]


def codes_missing_financial_reports(conn: sqlite3.Connection, report_limit: int) -> list[str]:
    periods = latest_report_candidates(report_limit)
    if not periods:
        return []
    placeholders = ",".join("?" for _ in periods)
    try:
        rows = conn.execute(
            f"""
            SELECT s.code
            FROM factor_securities s
            WHERE s.is_active = 1 AND s.is_st = 0 AND s.board IN ('MAIN', 'CHINEXT')
              AND NOT EXISTS (
                SELECT 1 FROM factor_financial_metrics f
                WHERE f.code = s.code AND f.report_period IN ({placeholders})
              )
            ORDER BY s.code ASC
            """,
            periods,
        ).fetchall()
    except sqlite3.Error:
        return active_security_codes(conn)
    return [str(row[0]) for row in rows]


def codes_missing_dividend_yield(conn: sqlite3.Connection) -> list[str]:
    try:
        rows = conn.execute(
            """
            SELECT d.code
            FROM factor_dividend_records d
            JOIN factor_securities s ON s.code = d.code
            WHERE s.is_active = 1 AND s.is_st = 0 AND s.board IN ('MAIN', 'CHINEXT')
            GROUP BY d.code
            HAVING SUM(CASE WHEN d.dividend_yield IS NOT NULL OR d.cash_dividend_per_share IS NOT NULL OR d.raw_plan <> '' THEN 1 ELSE 0 END) = 0
            ORDER BY d.code ASC
            """
        ).fetchall()
    except sqlite3.Error:
        return active_security_codes(conn)
    return [str(row[0]) for row in rows]


def codes_with_stale_dividends(conn: sqlite3.Connection, stale_days: int = 30) -> list[str]:
    cutoff = (datetime.utcnow() - timedelta(days=stale_days)).strftime("%Y-%m-%dT%H:%M:%S")
    try:
        rows = conn.execute(
            """
            SELECT s.code
            FROM factor_securities s
            LEFT JOIN (
              SELECT code, MAX(updated_at) AS max_updated_at
              FROM factor_dividend_records
              GROUP BY code
            ) d ON d.code = s.code
            WHERE s.is_active = 1 AND s.is_st = 0 AND s.board IN ('MAIN', 'CHINEXT')
              AND COALESCE(d.max_updated_at, '') < ?
            ORDER BY s.code ASC
            """,
            (cutoff,),
        ).fetchall()
    except sqlite3.Error:
        return active_security_codes(conn)
    return [str(row[0]) for row in rows]


def write_code_list(codes: list[str], mode: str) -> str:
    fd, path = tempfile.mkstemp(prefix=f"factorlab_{mode}_", suffix=".txt")
    with os.fdopen(fd, "w", encoding="utf-8") as handle:
        for code in codes:
            handle.write(f"{code}\n")
    return path


def build_mode_command(args: argparse.Namespace, mode: str, conn: sqlite3.Connection) -> list[str]:
    progress_interval = args.item_progress_interval if mode in {"daily-bars", "financials", "dividends"} else args.progress_interval
    cmd = [
        args.python_bin,
        str(BACKFILL_SCRIPT),
        "--db", str(args.db_path),
        "--mode", mode,
        "--progress-interval", str(progress_interval),
        "--sleep", str(args.sleep),
    ]
    if args.write:
        cmd.append("--write")
    if args.snapshot_date:
        cmd.extend(["--snapshot-date", args.snapshot_date])

    if mode == "daily-bars":
        target_date = latest_date(conn, "factor_market_metrics", "trade_date") or latest_date(conn, "factor_daily_bars", "trade_date")
        codes = codes_missing_daily_target(conn, target_date)
        cmd.extend(["--daily-bars-source", args.daily_bars_source])
        path = write_code_list(codes, mode)
        args.temp_files.append(path)
        cmd.extend(["--code-list-file", path])
        log_step(f"incremental: daily-bars target_date={target_date or 'unknown'} missing_codes={len(codes)} source={args.daily_bars_source}")
        start = buffered_start_date(latest_date(conn, "factor_daily_bars", "trade_date"), args.buffer_days)
        if start:
            cmd.extend(["--start-date", start])
        else:
            cmd.extend(["--lookback-days", str(args.lookback_days)])
    elif mode == "index-bars":
        start = buffered_start_date(latest_date(conn, "factor_index_daily_bars", "trade_date", "index_code = '000985'"), args.buffer_days)
        if start:
            cmd.extend(["--start-date", start])
        else:
            cmd.extend(["--lookback-days", str(args.lookback_days)])
    elif mode == "financials":
        if args.scope == "repair_missing_fcfm_inputs":
            codes = codes_missing_fcfm_inputs(conn)
            cmd.append("--require-fcfm-inputs")
            log_step(f"incremental: financials repair_missing_fcfm_inputs codes={len(codes)} source={args.financials_source}")
        else:
            codes = codes_missing_financial_reports(conn, args.financial_report_limit)
            log_step(f"incremental: financials missing_codes={len(codes)} source={args.financials_source}")
        cmd.extend(["--financials-source", args.financials_source, "--report-limit", str(args.financial_report_limit)])
        path = write_code_list(codes, mode)
        args.temp_files.append(path)
        cmd.extend(["--code-list-file", path])
    elif mode == "dividends":
        if args.scope == "repair_missing_dividend_yield":
            codes = codes_missing_dividend_yield(conn)
            log_step(f"incremental: dividends repair_missing_dividend_yield codes={len(codes)} source={args.dividends_source}")
        else:
            codes = codes_with_stale_dividends(conn)
            log_step(f"incremental: dividends stale_codes={len(codes)} source={args.dividends_source}")
        cmd.extend(["--dividends-source", args.dividends_source, "--report-limit", str(args.dividend_report_limit)])
        path = write_code_list(codes, mode)
        args.temp_files.append(path)
        cmd.extend(["--code-list-file", path])
    return cmd


def run_mode(args: argparse.Namespace, mode: str, conn: sqlite3.Connection) -> ModeResult:
    started = datetime.now()
    cmd = build_mode_command(args, mode, conn)
    log_step(f"incremental: 开始 {mode}: {' '.join(cmd)}")
    try:
        completed = subprocess.run(cmd, cwd=str(SCRIPT_DIR.parents[1]), text=True, timeout=args.step_timeout_seconds, check=False)
    except subprocess.TimeoutExpired:
        duration = (datetime.now() - started).total_seconds()
        message = f"timeout after {args.step_timeout_seconds}s"
        log_step(f"incremental: {mode} 超时 {message}")
        return ModeResult(mode=mode, status="failed", return_code=124, duration_seconds=duration, error=message)

    duration = (datetime.now() - started).total_seconds()
    status = "success" if completed.returncode == 0 else "failed"
    log_step(f"incremental: {mode} 完成 status={status} duration={duration:.1f}s")
    return ModeResult(mode=mode, status=status, return_code=completed.returncode, duration_seconds=duration, error="" if completed.returncode == 0 else f"exit {completed.returncode}")


def main(argv: list[str]) -> int:
    args = parse_args(argv)
    if args.lookback_days <= 0:
        raise ValueError("--lookback-days 必须大于 0")
    if args.buffer_days < 0:
        raise ValueError("--buffer-days 不能为负数")
    if args.financial_report_limit <= 0 or args.dividend_report_limit <= 0:
        raise ValueError("report limit 必须大于 0")
    if args.step_timeout_seconds <= 0:
        raise ValueError("--step-timeout-seconds 必须大于 0")
    if args.progress_interval <= 0:
        raise ValueError("--progress-interval 必须大于 0")
    if args.item_progress_interval <= 0:
        raise ValueError("--item-progress-interval 必须大于 0")
    args.temp_files = []

    modes = split_csv(args.modes)
    if modes == ["all"] or not modes:
        modes = list(DEFAULT_MODES)
    unsupported = [mode for mode in modes if mode not in DEFAULT_MODES]
    if unsupported:
        raise ValueError("不支持的增量模式: " + ",".join(unsupported))
    critical_modes = set(split_csv(args.critical_modes))

    args.db_path = resolve_db_path(args.db)
    conn = connect_db(args.db_path)
    ensure_schema(conn)
    print(f"db={args.db_path}", flush=True)
    print("mode=dry-run" if not args.write else "mode=write", flush=True)
    log_step(f"启动 Phase 0 incremental：modes={','.join(modes)}")

    results: list[ModeResult] = []
    try:
        for mode in modes:
            results.append(run_mode(args, mode, conn))
        failed = [item for item in results if item.status != "success"]
        critical_failed = [item for item in failed if item.mode in critical_modes]
        status = "success" if not failed else ("failed" if critical_failed else "partial")
        summary = {
            "status": status,
            "total": len(results),
            "success": sum(1 for item in results if item.status == "success"),
            "failed": len(failed),
            "results": [item.__dict__ for item in results],
        }
        print(f"summary={json.dumps(summary, ensure_ascii=False)} status={status}", flush=True)
        return 0 if status in {"success", "partial"} else 1
    finally:
        for path in getattr(args, "temp_files", []):
            try:
                os.remove(path)
            except OSError:
                pass
        conn.close()


if __name__ == "__main__":
    raise SystemExit(main(sys.argv[1:]))
