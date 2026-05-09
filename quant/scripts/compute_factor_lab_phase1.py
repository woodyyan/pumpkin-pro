#!/usr/bin/env python3
"""Factor Lab Phase 1 daily factor snapshot computation.

Phase 1 intentionally reads only local structured tables created/populated by
Phase 0. It does not call external market or financial data sources.
"""

from __future__ import annotations

import argparse
import json
import math
import sqlite3
import sys
import uuid
from dataclasses import dataclass
from datetime import datetime, timezone
from pathlib import Path
from typing import Any, Optional

SCRIPT_DIR = Path(__file__).resolve().parent
if str(SCRIPT_DIR) not in sys.path:
    sys.path.insert(0, str(SCRIPT_DIR))

from backfill_factor_lab_phase0 import (  # noqa: E402
    TaskStats,
    connect_db,
    ensure_schema,
    infer_symbol,
    log_progress,
    log_step,
    normalize_code,
    resolve_db_path,
    safe_float,
    upsert_task_item,
    utc_now,
    was_successful,
)

TASK_TYPE_DAILY_COMPUTE = "daily_compute"
TASK_STATUS_RUNNING = "running"
TASK_STATUS_SUCCESS = "success"
TASK_STATUS_PARTIAL = "partial"
TASK_STATUS_FAILED = "failed"
DEFAULT_INDEX_CODE = "000985"
MIN_BETA_SAMPLE_DAYS = 180
NEW_STOCK_DAYS = 365


@dataclass
class UniverseSecurity:
    code: str
    symbol: str
    name: str
    board: str
    listing_date: str


@dataclass
class MarketMetric:
    close_price: float
    market_cap: Optional[float]
    pe: Optional[float]
    pb: Optional[float]
    volume: float
    amount: float
    turnover_rate: Optional[float]
    is_suspended: bool
    trade_date: str


@dataclass
class FinancialMetric:
    report_period: str
    revenue: Optional[float]
    revenue_yoy: Optional[float]
    net_profit: Optional[float]
    net_profit_yoy: Optional[float]
    total_assets: Optional[float]
    total_equity: Optional[float]
    operating_cash_flow: Optional[float]


@dataclass
class DividendRecord:
    report_period: str
    ex_dividend_date: str
    cash_dividend_per_share: Optional[float]
    total_cash_dividend: Optional[float]


@dataclass
class DailyBar:
    trade_date: str
    close: float
    volume: float
    amount: float


@dataclass
class SnapshotResult:
    row: tuple[Any, ...]
    flags: list[str]


@dataclass
class CoverageSummary:
    total: int
    included: int
    metrics: dict[str, int]
    excluded: dict[str, int]


def parse_date(value: str) -> Optional[datetime]:
    text = str(value or "").strip()
    if not text:
        return None
    try:
        return datetime.strptime(text[:10], "%Y-%m-%d")
    except ValueError:
        return None


def percent_return(current: float, base: float) -> Optional[float]:
    if current <= 0 or base <= 0:
        return None
    return (current / base - 1.0) * 100.0


def safe_ratio(numerator: Optional[float], denominator: Optional[float], scale: float = 1.0) -> Optional[float]:
    if numerator is None or denominator is None or denominator == 0:
        return None
    value = numerator / denominator * scale
    if math.isnan(value) or math.isinf(value):
        return None
    return value


def latest_trade_date(conn: sqlite3.Connection) -> str:
    # Prefer actual daily bars. Market metrics may be stamped with a calendar day
    # when the offline job runs, including weekends, while factor computation
    # requires the snapshot date to exist in factor_daily_bars.
    row = conn.execute("SELECT MAX(trade_date) FROM factor_daily_bars").fetchone()
    if row and row[0]:
        return row[0]
    row = conn.execute("SELECT MAX(trade_date) FROM factor_market_metrics").fetchone()
    return row[0] if row and row[0] else ""


def insert_task_run(conn: sqlite3.Connection, run_id: str, args: argparse.Namespace) -> None:
    conn.execute(
        """
        INSERT OR REPLACE INTO factor_task_runs
        (id, task_type, snapshot_date, status, started_at, total_count, success_count, failed_count, skipped_count, params_json, summary_json, error_message)
        VALUES (?, ?, ?, ?, ?, 0, 0, 0, 0, ?, '{}', '')
        """,
        (
            run_id,
            TASK_TYPE_DAILY_COMPUTE,
            args.snapshot_date,
            TASK_STATUS_RUNNING,
            utc_now(),
            json.dumps(vars(args), ensure_ascii=False, default=str),
        ),
    )
    conn.commit()


def finish_task_run(conn: sqlite3.Connection, run_id: str, status: str, stats: TaskStats, summary: dict[str, Any], error: str = "") -> None:
    conn.execute(
        """
        UPDATE factor_task_runs
        SET status = ?, finished_at = ?, total_count = ?, success_count = ?, failed_count = ?, skipped_count = ?, summary_json = ?, error_message = ?
        WHERE id = ?
        """,
        (
            status,
            utc_now(),
            stats.total,
            stats.success,
            stats.failed,
            stats.skipped,
            json.dumps(summary, ensure_ascii=False),
            error,
            run_id,
        ),
    )
    conn.commit()


def load_universe(conn: sqlite3.Connection, args: argparse.Namespace) -> list[UniverseSecurity]:
    params: list[Any] = []
    where = ["is_active = 1", "is_st = 0", "board IN ('MAIN', 'CHINEXT')"]
    if args.code:
        where.append("code = ?")
        params.append(normalize_code(args.code))
    query = f"""
        SELECT code, symbol, name, board, listing_date
        FROM factor_securities
        WHERE {' AND '.join(where)}
        ORDER BY code ASC
    """
    if args.limit > 0:
        query += " LIMIT ?"
        params.append(args.limit)
    rows = conn.execute(query, params).fetchall()
    return [
        UniverseSecurity(
            code=normalize_code(code),
            symbol=symbol or infer_symbol(code),
            name=name or normalize_code(code),
            board=board or "",
            listing_date=listing_date or "",
        )
        for code, symbol, name, board, listing_date in rows
    ]


def load_market_metric(conn: sqlite3.Connection, code: str, snapshot_date: str) -> Optional[MarketMetric]:
    row = conn.execute(
        """
        SELECT trade_date, close_price, market_cap, pe, pb, volume, amount, turnover_rate, is_suspended
        FROM factor_market_metrics
        WHERE code = ? AND trade_date <= ?
        ORDER BY trade_date DESC
        LIMIT 1
        """,
        (code, snapshot_date),
    ).fetchone()
    if not row:
        return None
    trade_date, close_price, market_cap, pe, pb, volume, amount, turnover_rate, is_suspended = row
    return MarketMetric(
        trade_date=trade_date,
        close_price=safe_float(close_price) or 0.0,
        market_cap=safe_float(market_cap),
        pe=safe_float(pe),
        pb=safe_float(pb),
        volume=safe_float(volume) or 0.0,
        amount=safe_float(amount) or 0.0,
        turnover_rate=safe_float(turnover_rate),
        is_suspended=bool(is_suspended),
    )


def load_daily_bars(conn: sqlite3.Connection, code: str, snapshot_date: str) -> list[DailyBar]:
    rows = conn.execute(
        """
        SELECT trade_date, close, volume, amount
        FROM factor_daily_bars
        WHERE code = ? AND trade_date <= ? AND close > 0
        ORDER BY trade_date ASC
        """,
        (code, snapshot_date),
    ).fetchall()
    return [DailyBar(trade_date=d, close=float(c), volume=float(v or 0), amount=float(a or 0)) for d, c, v, a in rows]


def load_index_returns(conn: sqlite3.Connection, index_code: str, snapshot_date: str) -> dict[str, float]:
    rows = conn.execute(
        """
        SELECT trade_date, pct_change
        FROM factor_index_daily_bars
        WHERE index_code = ? AND trade_date <= ? AND pct_change IS NOT NULL
        ORDER BY trade_date ASC
        """,
        (index_code, snapshot_date),
    ).fetchall()
    return {d: float(pct) / 100.0 for d, pct in rows if safe_float(pct) is not None}


def load_latest_financial(conn: sqlite3.Connection, code: str, snapshot_date: str) -> Optional[FinancialMetric]:
    row = conn.execute(
        """
        SELECT report_period, revenue, revenue_yoy, net_profit, net_profit_yoy, total_assets, total_equity, operating_cash_flow
        FROM factor_financial_metrics
        WHERE code = ? AND report_period <= ?
        ORDER BY report_period DESC
        LIMIT 1
        """,
        (code, snapshot_date),
    ).fetchone()
    if not row:
        return None
    return FinancialMetric(
        report_period=row[0],
        revenue=safe_float(row[1]),
        revenue_yoy=safe_float(row[2]),
        net_profit=safe_float(row[3]),
        net_profit_yoy=safe_float(row[4]),
        total_assets=safe_float(row[5]),
        total_equity=safe_float(row[6]),
        operating_cash_flow=safe_float(row[7]),
    )


def load_latest_dividend(conn: sqlite3.Connection, code: str, snapshot_date: str) -> Optional[DividendRecord]:
    row = conn.execute(
        """
        SELECT report_period, ex_dividend_date, cash_dividend_per_share, total_cash_dividend
        FROM factor_dividend_records
        WHERE code = ? AND (ex_dividend_date <= ? OR ex_dividend_date = 'unknown')
        ORDER BY CASE WHEN ex_dividend_date = 'unknown' THEN report_period ELSE ex_dividend_date END DESC
        LIMIT 1
        """,
        (code, snapshot_date),
    ).fetchone()
    if not row:
        return None
    return DividendRecord(
        report_period=row[0],
        ex_dividend_date=row[1],
        cash_dividend_per_share=safe_float(row[2]),
        total_cash_dividend=safe_float(row[3]),
    )


def daily_returns(bars: list[DailyBar]) -> list[tuple[str, float]]:
    result: list[tuple[str, float]] = []
    prev = None
    for bar in bars:
        if prev and prev.close > 0 and bar.close > 0:
            result.append((bar.trade_date, bar.close / prev.close - 1.0))
        prev = bar
    return result


def volatility_annualized(returns: list[float]) -> Optional[float]:
    if len(returns) < 2:
        return None
    mean = sum(returns) / len(returns)
    variance = sum((item - mean) ** 2 for item in returns) / (len(returns) - 1)
    return math.sqrt(variance) * math.sqrt(252) * 100.0


def beta_against_index(stock_returns: list[tuple[str, float]], index_returns: dict[str, float], min_samples: int = MIN_BETA_SAMPLE_DAYS) -> tuple[Optional[float], int]:
    pairs = [(ret, index_returns[date]) for date, ret in stock_returns if date in index_returns]
    if len(pairs) < min_samples:
        return None, len(pairs)
    stock_values = [item[0] for item in pairs]
    index_values = [item[1] for item in pairs]
    stock_mean = sum(stock_values) / len(stock_values)
    index_mean = sum(index_values) / len(index_values)
    variance = sum((item - index_mean) ** 2 for item in index_values)
    if variance == 0:
        return None, len(pairs)
    covariance = sum((stock_values[i] - stock_mean) * (index_values[i] - index_mean) for i in range(len(pairs)))
    return covariance / variance, len(pairs)


def compute_listing_age_days(listing_date: str, snapshot_date: str) -> Optional[int]:
    listing = parse_date(listing_date)
    snapshot = parse_date(snapshot_date)
    if not listing or not snapshot:
        return None
    return max((snapshot - listing).days, 0)


def compute_dividend_yield(dividend: Optional[DividendRecord], market_cap: Optional[float], close_price: float) -> Optional[float]:
    if not dividend:
        return None
    if dividend.total_cash_dividend is not None and market_cap and market_cap > 0:
        return dividend.total_cash_dividend / market_cap
    if dividend.cash_dividend_per_share is not None and close_price > 0:
        return dividend.cash_dividend_per_share / close_price
    return None


def compute_snapshot_for_security(
    security: UniverseSecurity,
    market: Optional[MarketMetric],
    bars: list[DailyBar],
    financial: Optional[FinancialMetric],
    dividend: Optional[DividendRecord],
    index_returns: dict[str, float],
    snapshot_date: str,
) -> tuple[Optional[SnapshotResult], str]:
    flags: list[str] = []
    if not bars:
        return None, "no_daily_bars"
    latest_bar = bars[-1]
    if latest_bar.trade_date != snapshot_date:
        return None, "no_snapshot_date_bar"
    if latest_bar.close <= 0 or latest_bar.volume <= 0:
        return None, "suspended_or_invalid_daily_bar"
    if market and (market.is_suspended or market.close_price <= 0 or market.volume <= 0):
        return None, "suspended_market_metric"

    close_price = market.close_price if market and market.close_price > 0 else latest_bar.close
    market_cap = market.market_cap if market else None
    pe = market.pe if market else None
    pb = market.pb if market else None
    available_days = len(bars)
    listing_age_days = compute_listing_age_days(security.listing_date, snapshot_date)
    if listing_age_days is None:
        flags.append("no_listing_date")
    is_new_stock = bool(listing_age_days is not None and listing_age_days < NEW_STOCK_DAYS)

    performance_since_listing = percent_return(latest_bar.close, bars[0].close) if len(bars) >= 2 else None
    if len(bars) >= 250:
        performance_1y = percent_return(latest_bar.close, bars[-250].close)
    else:
        performance_1y = None
        flags.append("insufficient_1y_bars")
    if len(bars) >= 21:
        momentum_1m = percent_return(latest_bar.close, bars[-21].close)
    else:
        momentum_1m = None
        flags.append("insufficient_1m_bars")

    returns = daily_returns(bars)
    recent_returns = [item[1] for item in returns[-20:]]
    volatility_1m = volatility_annualized(recent_returns)
    if volatility_1m is None:
        flags.append("insufficient_volatility_samples")
    beta_1y, beta_samples = beta_against_index(returns[-260:], index_returns)
    if beta_1y is None:
        flags.append(f"insufficient_beta_samples:{beta_samples}")

    if financial is None:
        flags.append("no_financial")
        ps = earning_growth = revenue_growth = roe = operating_cf_margin = asset_to_equity = None
    else:
        ps = safe_ratio(market_cap, financial.revenue)
        earning_growth = financial.net_profit_yoy
        revenue_growth = financial.revenue_yoy
        roe = safe_ratio(financial.net_profit, financial.total_equity, 100.0)
        operating_cf_margin = safe_ratio(financial.operating_cash_flow, financial.revenue, 100.0)
        asset_to_equity = safe_ratio(financial.total_assets, financial.total_equity)
        if ps is None:
            flags.append("no_ps")
        if roe is None:
            flags.append("no_roe")
        if operating_cf_margin is None:
            flags.append("no_operating_cf_margin")
        if asset_to_equity is None:
            flags.append("no_asset_to_equity")

    dividend_yield = compute_dividend_yield(dividend, market_cap, close_price)
    if dividend_yield is None:
        flags.append("no_dividend_yield")
    if market_cap is None:
        flags.append("no_market_cap")
    if pe is None:
        flags.append("no_pe")
    if pb is None:
        flags.append("no_pb")

    row = (
        snapshot_date,
        security.code,
        security.symbol or infer_symbol(security.code),
        security.name,
        security.board,
        listing_age_days,
        int(is_new_stock),
        available_days,
        close_price,
        market_cap,
        pe,
        pb,
        ps,
        dividend_yield,
        earning_growth,
        revenue_growth,
        performance_1y,
        performance_since_listing,
        momentum_1m,
        roe,
        operating_cf_margin,
        asset_to_equity,
        volatility_1m,
        beta_1y,
        json.dumps(flags, ensure_ascii=False),
        utc_now(),
    )
    return SnapshotResult(row=row, flags=flags), "included"


def metric_coverage(rows: list[tuple[Any, ...]]) -> dict[str, int]:
    # Offsets match factor_snapshots insert column order below.
    mapping = {
        "market_cap": 9,
        "pe": 10,
        "pb": 11,
        "ps": 12,
        "dividend_yield": 13,
        "earning_growth": 14,
        "revenue_growth": 15,
        "performance_1y": 16,
        "performance_since_listing": 17,
        "momentum_1m": 18,
        "roe": 19,
        "operating_cf_margin": 20,
        "asset_to_equity": 21,
        "volatility_1m": 22,
        "beta_1y": 23,
    }
    return {key: sum(1 for row in rows if row[idx] is not None) for key, idx in mapping.items()}


def insert_snapshots(conn: sqlite3.Connection, rows: list[tuple[Any, ...]]) -> None:
    conn.executemany(
        """
        INSERT OR REPLACE INTO factor_snapshots
        (snapshot_date, code, symbol, name, board, listing_age_days, is_new_stock, available_trading_days,
         close_price, market_cap, pe, pb, ps, dividend_yield, earning_growth, revenue_growth,
         performance_1y, performance_since_listing, momentum_1m, roe, operating_cf_margin, asset_to_equity,
         volatility_1m, beta_1y, data_quality_flags, created_at)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
        """,
        rows,
    )


def compute_snapshots(conn: sqlite3.Connection, args: argparse.Namespace, run_id: str) -> tuple[list[tuple[Any, ...]], CoverageSummary]:
    universe = load_universe(conn, args)
    index_returns = load_index_returns(conn, args.index_code, args.snapshot_date)
    rows: list[tuple[Any, ...]] = []
    excluded: dict[str, int] = {}
    log_step(f"phase1: universe={len(universe)} index_return_days={len(index_returns)} snapshot_date={args.snapshot_date}")
    for idx, security in enumerate(universe, start=1):
        log_progress("phase1: 计算进度", idx, len(universe), args.progress_interval)
        item_key = f"snapshot:{args.snapshot_date}:{security.code}"
        if args.resume and was_successful(conn, "daily_compute", item_key):
            continue
        market = load_market_metric(conn, security.code, args.snapshot_date)
        bars = load_daily_bars(conn, security.code, args.snapshot_date)
        financial = load_latest_financial(conn, security.code, args.snapshot_date)
        dividend = load_latest_dividend(conn, security.code, args.snapshot_date)
        result, reason = compute_snapshot_for_security(security, market, bars, financial, dividend, index_returns, args.snapshot_date)
        if result is None:
            excluded[reason] = excluded.get(reason, 0) + 1
            if args.write:
                upsert_task_item(conn, run_id, "daily_compute", item_key, TASK_STATUS_FAILED, reason)
            continue
        rows.append(result.row)
        if args.write:
            upsert_task_item(conn, run_id, "daily_compute", item_key, TASK_STATUS_SUCCESS)
    coverage = CoverageSummary(total=len(universe), included=len(rows), metrics=metric_coverage(rows), excluded=excluded)
    return rows, coverage


def run_daily_compute(conn: sqlite3.Connection, args: argparse.Namespace, run_id: str) -> TaskStats:
    rows, coverage = compute_snapshots(conn, args, run_id)
    stats = TaskStats(total=coverage.total, success=len(rows), failed=coverage.total - len(rows), skipped=0)
    summary = {
        "snapshot_date": args.snapshot_date,
        "universe_count": coverage.total,
        "included_count": coverage.included,
        "excluded": coverage.excluded,
        "coverage": coverage.metrics,
    }
    log_step(f"phase1: 计算完成 included={coverage.included}/{coverage.total} excluded={coverage.excluded}")
    log_step(f"phase1: 指标覆盖 {json.dumps(coverage.metrics, ensure_ascii=False)}")
    if not args.write:
        print(f"[dry-run] snapshots={len(rows)} summary={json.dumps(summary, ensure_ascii=False)}", flush=True)
        stats.skipped = len(rows)
        stats.success = 0
        stats.failed = 0
        return stats
    insert_snapshots(conn, rows)
    conn.commit()
    log_step(f"phase1: 写入 factor_snapshots {len(rows)} 条")
    return stats


def parse_args(argv: list[str]) -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Factor Lab Phase 1 daily factor snapshot computation")
    parser.add_argument("--db", default="", help="pumpkin.db 路径；默认自动查找 data/pumpkin.db")
    parser.add_argument("--snapshot-date", default="", help="快照交易日 YYYY-MM-DD；默认取本地最新交易日")
    parser.add_argument("--index-code", default=DEFAULT_INDEX_CODE, help="Beta 基准指数，默认中证全指 000985")
    parser.add_argument("--write", action="store_true", help="实际写入 factor_snapshots；默认 dry-run")
    parser.add_argument("--resume", action="store_true", help="跳过已有 success task item")
    parser.add_argument("--limit", type=int, default=0, help="最多处理多少只股票，0 表示不限制")
    parser.add_argument("--code", default="", help="只处理单只股票代码")
    parser.add_argument("--progress-interval", type=int, default=100, help="每处理多少只股票输出一次进度")
    return parser.parse_args(argv)


def main(argv: list[str]) -> int:
    args = parse_args(argv)
    if args.limit < 0:
        raise ValueError("--limit 不能为负数")
    if args.progress_interval <= 0:
        raise ValueError("--progress-interval 必须大于 0")
    db_path = resolve_db_path(args.db)
    conn = connect_db(db_path)
    ensure_schema(conn)
    if not args.snapshot_date:
        args.snapshot_date = latest_trade_date(conn)
    if not args.snapshot_date:
        print("failed: 本地 factor_daily_bars/factor_market_metrics 没有可用交易日", file=sys.stderr)
        return 1
    run_id = str(uuid.uuid4())
    print(f"db={db_path}", flush=True)
    print(f"run_id={run_id}", flush=True)
    print("mode=dry-run" if not args.write else "mode=write", flush=True)
    log_step(f"启动 Phase 1 daily_compute：snapshot_date={args.snapshot_date}")
    if args.write:
        insert_task_run(conn, run_id, args)
    try:
        stats = run_daily_compute(conn, args, run_id)
        status = TASK_STATUS_SUCCESS if stats.failed == 0 else (TASK_STATUS_PARTIAL if stats.success > 0 else TASK_STATUS_FAILED)
        summary = {"total": stats.total, "success": stats.success, "failed": stats.failed, "skipped": stats.skipped, "snapshot_date": args.snapshot_date}
        if args.write:
            finish_task_run(conn, run_id, status, stats, summary)
        print(f"summary={json.dumps(summary, ensure_ascii=False)} status={status}", flush=True)
        return 0 if status in {TASK_STATUS_SUCCESS, TASK_STATUS_PARTIAL} else 1
    except Exception as exc:  # noqa: BLE001
        if args.write:
            finish_task_run(conn, run_id, TASK_STATUS_FAILED, TaskStats(), {}, str(exc))
        print(f"failed: {exc}", file=sys.stderr)
        return 1
    finally:
        conn.close()


if __name__ == "__main__":
    raise SystemExit(main(sys.argv[1:]))
