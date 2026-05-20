#!/usr/bin/env python3
"""Factor Lab Phase 0 daily incremental updater.

This command is intentionally separate from the one-off backfill command. It is
for production daily jobs: refresh the smallest useful data window, keep writes
idempotent, and continue non-critical modes when possible.
"""

from __future__ import annotations

import argparse
import json
import sqlite3
import subprocess
import sys
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
    parser.add_argument("--critical-modes", default=",".join(sorted(DEFAULT_CRITICAL_MODES)), help="失败后必须返回 failed 的关键模式")
    parser.add_argument("--lookback-days", type=int, default=DEFAULT_LOOKBACK_DAYS, help="无本地日期时的兜底回看自然日")
    parser.add_argument("--buffer-days", type=int, default=DEFAULT_BUFFER_DAYS, help="从本地最新日期向前回退的增量缓冲天数")
    parser.add_argument("--financial-report-limit", type=int, default=DEFAULT_FINANCIAL_REPORT_LIMIT, help="财务增量扫描最近报告期数量")
    parser.add_argument("--dividend-report-limit", type=int, default=DEFAULT_DIVIDEND_REPORT_LIMIT, help="分红增量扫描最近报告期数量")
    parser.add_argument("--step-timeout-seconds", type=int, default=DEFAULT_STEP_TIMEOUT_SECONDS, help="每个子任务超时时间")
    parser.add_argument("--progress-interval", type=int, default=500, help="每处理多少项输出一次进度")
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


def build_mode_command(args: argparse.Namespace, mode: str, conn: sqlite3.Connection) -> list[str]:
    cmd = [
        args.python_bin,
        str(BACKFILL_SCRIPT),
        "--db", str(args.db_path),
        "--mode", mode,
        "--progress-interval", str(args.progress_interval),
        "--sleep", str(args.sleep),
    ]
    if args.write:
        cmd.append("--write")
    if args.snapshot_date:
        cmd.extend(["--snapshot-date", args.snapshot_date])

    if mode == "daily-bars":
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
        cmd.extend(["--report-limit", str(args.financial_report_limit)])
    elif mode == "dividends":
        cmd.extend(["--report-limit", str(args.dividend_report_limit)])
    return cmd


def run_mode(args: argparse.Namespace, mode: str, conn: sqlite3.Connection) -> ModeResult:
    started = datetime.now()
    cmd = build_mode_command(args, mode, conn)
    log_step(f"incremental: 开始 {mode}: {' '.join(cmd)}")
    try:
        completed = subprocess.run(cmd, cwd=str(SCRIPT_DIR.parents[1]), text=True, capture_output=True, timeout=args.step_timeout_seconds, check=False)
    except subprocess.TimeoutExpired as exc:
        duration = (datetime.now() - started).total_seconds()
        message = f"timeout after {args.step_timeout_seconds}s"
        if exc.stdout:
            print(exc.stdout, flush=True)
        if exc.stderr:
            print(exc.stderr, file=sys.stderr, flush=True)
        log_step(f"incremental: {mode} 超时 {message}")
        return ModeResult(mode=mode, status="failed", return_code=124, duration_seconds=duration, error=message)

    if completed.stdout:
        print(completed.stdout, end="", flush=True)
    if completed.stderr:
        print(completed.stderr, end="", file=sys.stderr, flush=True)
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

    modes = split_csv(args.modes)
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
        conn.close()


if __name__ == "__main__":
    raise SystemExit(main(sys.argv[1:]))
