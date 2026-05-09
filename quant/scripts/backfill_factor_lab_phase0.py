#!/usr/bin/env python3
"""Factor Lab Phase 0 one-off backfill task.

This command is intentionally project-owned rather than an ad-hoc local script:
- It creates/validates the Phase 0 tables when needed.
- It supports dry-run by default, --write for actual writes, --limit for sampling,
  --code for single-symbol debugging, and --resume to skip successful task items.
- It may access external data sources because Phase 0 is an offline backfill task.

User-facing Factor Lab queries and Phase 1 daily factor computation should read only
from the local structured tables populated by this command.
"""

from __future__ import annotations

import argparse
import json
import math
import sqlite3
import sys
import time
import traceback
import uuid
from dataclasses import dataclass
from datetime import datetime, timedelta, timezone
from pathlib import Path
from typing import Any, Iterable, Optional

PROJECT_ROOT = Path(__file__).resolve().parents[2]
DEFAULT_DB_CANDIDATES = [
    PROJECT_ROOT / "data" / "pumpkin.db",
    PROJECT_ROOT / "backend" / "data" / "pumpkin.db",
]
DEFAULT_INDEX_CODE = "000985"  # 中证全指
DEFAULT_LOOKBACK_DAYS = 370    # calendar days, usually enough for ~250 trading days
DEFAULT_ADJUST = "qfq"
EXTERNAL_SOURCE_ORDER = ["akshare", "eastmoney", "tencent"]
SECURITIES_SOURCE_ORDER = ["akshare", "eastmoney", "tencent", "local"]
EASTMONEY_CLIST_URL = "https://82.push2.eastmoney.com/api/qt/clist/get"
EASTMONEY_KLINE_URL = "https://push2his.eastmoney.com/api/qt/stock/kline/get"
EASTMONEY_DATACENTER_URL = "https://datacenter-web.eastmoney.com/api/data/v1/get"
TENCENT_QUOTE_URL = "http://qt.gtimg.cn/q={codes}"
TENCENT_KLINE_URL = "https://web.ifzq.gtimg.cn/appstock/app/fqkline/get"

CODE_ALIASES = ["股票代码", "代码", "证券代码"]
NAME_ALIASES = ["股票简称", "名称", "股票名称"]
REVENUE_ALIASES = ["营业总收入-营业总收入", "营业收入-营业收入", "营业总收入", "营业收入"]
REVENUE_YOY_ALIASES = ["营业总收入-同比增长", "营业收入-同比增长", "营业收入同比增长", "营业总收入同比增长"]
NET_PROFIT_ALIASES = ["净利润-净利润", "归母净利润-净利润", "净利润", "归母净利润"]
NET_PROFIT_YOY_ALIASES = ["净利润-同比增长", "净利润同比增长", "归母净利润-同比增长"]
TOTAL_ASSETS_ALIASES = ["资产总计", "总资产", "资产总计-资产总计"]
TOTAL_EQUITY_ALIASES = ["所有者权益合计", "股东权益合计", "归属于母公司股东权益合计", "所有者权益(或股东权益)合计"]
OPERATING_CF_ALIASES = ["经营活动产生的现金流量净额", "经营现金流量净额", "经营活动现金流量净额"]
REPORT_DATE_ALIASES = ["最新公告日期", "公告日期", "业绩披露日期"]
DIVIDEND_PER_SHARE_ALIASES = ["每股现金红利", "派息", "现金分红-每股现金红利", "现金分红-派息比例"]
TOTAL_DIVIDEND_ALIASES = ["现金分红总额", "分红总额", "现金分红-现金分红总额"]
EX_DIVIDEND_DATE_ALIASES = ["除权除息日", "除息日", "股权登记日"]
REPORT_PERIOD_ALIASES = ["报告期", "分红年度"]

SCHEMA_SQL = [
    """
    CREATE TABLE IF NOT EXISTS factor_securities (
        code TEXT PRIMARY KEY,
        symbol TEXT NOT NULL DEFAULT '',
        name TEXT NOT NULL DEFAULT '',
        exchange TEXT NOT NULL DEFAULT '',
        board TEXT NOT NULL DEFAULT '',
        listing_date TEXT NOT NULL DEFAULT '',
        is_st INTEGER NOT NULL DEFAULT 0,
        is_active INTEGER NOT NULL DEFAULT 1,
        source TEXT NOT NULL DEFAULT '',
        updated_at DATETIME NOT NULL
    )
    """,
    "CREATE INDEX IF NOT EXISTS idx_factor_securities_exchange ON factor_securities(exchange)",
    "CREATE INDEX IF NOT EXISTS idx_factor_securities_board ON factor_securities(board)",
    "CREATE INDEX IF NOT EXISTS idx_factor_securities_is_st ON factor_securities(is_st)",
    "CREATE INDEX IF NOT EXISTS idx_factor_securities_is_active ON factor_securities(is_active)",
    """
    CREATE TABLE IF NOT EXISTS factor_daily_bars (
        code TEXT NOT NULL,
        trade_date TEXT NOT NULL,
        open REAL NOT NULL DEFAULT 0,
        close REAL NOT NULL DEFAULT 0,
        high REAL NOT NULL DEFAULT 0,
        low REAL NOT NULL DEFAULT 0,
        volume REAL NOT NULL DEFAULT 0,
        amount REAL NOT NULL DEFAULT 0,
        turnover_rate REAL,
        adjusted TEXT NOT NULL DEFAULT 'qfq',
        source TEXT NOT NULL DEFAULT '',
        updated_at DATETIME NOT NULL,
        PRIMARY KEY (code, trade_date)
    )
    """,
    """
    CREATE TABLE IF NOT EXISTS factor_index_daily_bars (
        index_code TEXT NOT NULL,
        trade_date TEXT NOT NULL,
        close REAL NOT NULL DEFAULT 0,
        pct_change REAL,
        source TEXT NOT NULL DEFAULT '',
        updated_at DATETIME NOT NULL,
        PRIMARY KEY (index_code, trade_date)
    )
    """,
    """
    CREATE TABLE IF NOT EXISTS factor_market_metrics (
        code TEXT NOT NULL,
        trade_date TEXT NOT NULL,
        close_price REAL NOT NULL DEFAULT 0,
        market_cap REAL,
        pe REAL,
        pb REAL,
        volume REAL NOT NULL DEFAULT 0,
        amount REAL NOT NULL DEFAULT 0,
        turnover_rate REAL,
        is_suspended INTEGER NOT NULL DEFAULT 0,
        source TEXT NOT NULL DEFAULT '',
        updated_at DATETIME NOT NULL,
        PRIMARY KEY (code, trade_date)
    )
    """,
    """
    CREATE TABLE IF NOT EXISTS factor_financial_metrics (
        code TEXT NOT NULL,
        report_period TEXT NOT NULL,
        report_date TEXT NOT NULL DEFAULT '',
        revenue REAL,
        revenue_yoy REAL,
        net_profit REAL,
        net_profit_yoy REAL,
        total_assets REAL,
        total_equity REAL,
        operating_cash_flow REAL,
        source TEXT NOT NULL DEFAULT '',
        updated_at DATETIME NOT NULL,
        PRIMARY KEY (code, report_period)
    )
    """,
    """
    CREATE TABLE IF NOT EXISTS factor_dividend_records (
        code TEXT NOT NULL,
        report_period TEXT NOT NULL,
        ex_dividend_date TEXT NOT NULL DEFAULT 'unknown',
        cash_dividend_per_share REAL,
        total_cash_dividend REAL,
        source TEXT NOT NULL DEFAULT '',
        updated_at DATETIME NOT NULL,
        PRIMARY KEY (code, report_period, ex_dividend_date)
    )
    """,
    """
    CREATE TABLE IF NOT EXISTS factor_snapshots (
        snapshot_date TEXT NOT NULL,
        code TEXT NOT NULL,
        symbol TEXT NOT NULL DEFAULT '',
        name TEXT NOT NULL DEFAULT '',
        board TEXT NOT NULL DEFAULT '',
        listing_age_days INTEGER,
        is_new_stock INTEGER NOT NULL DEFAULT 0,
        available_trading_days INTEGER NOT NULL DEFAULT 0,
        close_price REAL NOT NULL DEFAULT 0,
        market_cap REAL,
        pe REAL,
        pb REAL,
        ps REAL,
        dividend_yield REAL,
        earning_growth REAL,
        revenue_growth REAL,
        performance_1y REAL,
        performance_since_listing REAL,
        momentum_1m REAL,
        roe REAL,
        operating_cf_margin REAL,
        asset_to_equity REAL,
        volatility_1m REAL,
        beta_1y REAL,
        data_quality_flags TEXT NOT NULL DEFAULT '[]',
        created_at DATETIME NOT NULL,
        PRIMARY KEY (snapshot_date, code)
    )
    """,
    """
    CREATE TABLE IF NOT EXISTS factor_task_runs (
        id TEXT PRIMARY KEY,
        task_type TEXT NOT NULL,
        snapshot_date TEXT NOT NULL DEFAULT '',
        status TEXT NOT NULL,
        started_at DATETIME NOT NULL,
        finished_at DATETIME,
        total_count INTEGER NOT NULL DEFAULT 0,
        success_count INTEGER NOT NULL DEFAULT 0,
        failed_count INTEGER NOT NULL DEFAULT 0,
        skipped_count INTEGER NOT NULL DEFAULT 0,
        params_json TEXT NOT NULL DEFAULT '{}',
        summary_json TEXT NOT NULL DEFAULT '{}',
        error_message TEXT NOT NULL DEFAULT ''
    )
    """,
    "CREATE INDEX IF NOT EXISTS idx_factor_task_runs_task_type ON factor_task_runs(task_type)",
    "CREATE INDEX IF NOT EXISTS idx_factor_task_runs_snapshot_date ON factor_task_runs(snapshot_date)",
    "CREATE INDEX IF NOT EXISTS idx_factor_task_runs_status ON factor_task_runs(status)",
    """
    CREATE TABLE IF NOT EXISTS factor_task_items (
        run_id TEXT NOT NULL,
        item_type TEXT NOT NULL,
        item_key TEXT NOT NULL,
        status TEXT NOT NULL,
        error_message TEXT NOT NULL DEFAULT '',
        updated_at DATETIME NOT NULL,
        PRIMARY KEY (run_id, item_type, item_key)
    )
    """,
]


@dataclass
class TaskStats:
    total: int = 0
    success: int = 0
    failed: int = 0
    skipped: int = 0


@dataclass
class SecuritiesPayload:
    rows: list[tuple[Any, ...]]
    metrics: list[tuple[Any, ...]]
    source: str


def log_step(message: str) -> None:
    print(f"[{datetime.now().strftime('%H:%M:%S')}] {message}", flush=True)


def log_progress(label: str, current: int, total: int, interval: int) -> None:
    if total <= 0:
        return
    interval = max(interval, 1)
    if current == 1 or current == total or current % interval == 0:
        print(f"[{datetime.now().strftime('%H:%M:%S')}] {label}: {current}/{total}", flush=True)


def import_akshare():
    import akshare as ak  # type: ignore
    return ak


def import_pandas():
    import pandas as pd  # type: ignore
    return pd


def import_requests():
    import requests  # type: ignore
    return requests


def utc_now() -> str:
    return datetime.now(timezone.utc).replace(microsecond=0).isoformat().replace("+00:00", "Z")


def normalize_code(value: Any) -> str:
    text = str(value or "").strip().upper()
    if "." in text:
        text = text.split(".", 1)[0]
    digits = "".join(ch for ch in text if ch.isdigit())
    if not digits:
        return ""
    return digits.zfill(6)[-6:]


def infer_exchange(code: str) -> str:
    code = normalize_code(code)
    return "SSE" if code.startswith("6") else "SZSE"


def infer_symbol(code: str) -> str:
    code = normalize_code(code)
    if not code:
        return ""
    return f"{code}.SH" if infer_exchange(code) == "SSE" else f"{code}.SZ"


def classify_board(code: str) -> str:
    code = normalize_code(code)
    if code.startswith(("688", "689")):
        return "STAR"
    if code.startswith(("300", "301")):
        return "CHINEXT"
    if code.startswith(("8", "4", "920")):
        return "BJ"
    if code.startswith(("600", "601", "603", "605", "000", "001", "002", "003")):
        return "MAIN"
    return "OTHER"


def is_st_name(name: Any) -> bool:
    text = str(name or "").upper().replace(" ", "")
    return "ST" in text


def safe_float(value: Any) -> Optional[float]:
    if value is None:
        return None
    if isinstance(value, str) and value.strip() in {"", "-", "--", "None", "nan", "NaN"}:
        return None
    try:
        parsed = float(value)
    except (TypeError, ValueError):
        return None
    if math.isnan(parsed) or math.isinf(parsed):
        return None
    return parsed


def safe_scaled_float(value: Any, scale: float) -> Optional[float]:
    parsed = safe_float(value)
    if parsed is None:
        return None
    return parsed * scale


def normalize_date(value: Any) -> str:
    if value is None or value == "":
        return ""
    pd = import_pandas()
    try:
        ts = pd.to_datetime(value, errors="coerce")
    except Exception:
        return ""
    if pd.isna(ts):
        return ""
    return ts.strftime("%Y-%m-%d")


def find_column(columns: Iterable[Any], aliases: list[str]) -> Optional[str]:
    normalized = {str(col).strip(): col for col in columns}
    for alias in aliases:
        if alias in normalized:
            return normalized[alias]
    for alias in aliases:
        for key, original in normalized.items():
            if alias in key or key in alias:
                return original
    return None


def build_report_date_candidates(limit: int = 8) -> list[str]:
    today = datetime.today()
    candidates: list[str] = []
    for year in range(today.year, today.year - 3, -1):
        for month, day in ((12, 31), (9, 30), (6, 30), (3, 31)):
            dt = datetime(year, month, day)
            if dt > today:
                continue
            candidates.append(dt.strftime("%Y%m%d"))
            if len(candidates) >= limit:
                return candidates
    return candidates


def resolve_db_path(input_path: str) -> Path:
    if input_path:
        path = Path(input_path).expanduser().resolve()
        if not path.exists():
            raise FileNotFoundError(f"数据库文件不存在: {path}")
        return path
    for candidate in DEFAULT_DB_CANDIDATES:
        if candidate.exists():
            return candidate.resolve()
    raise FileNotFoundError("未找到 pumpkin.db，请通过 --db 显式指定")


def connect_db(path: Path) -> sqlite3.Connection:
    conn = sqlite3.connect(str(path), timeout=30)
    conn.execute("PRAGMA journal_mode=WAL")
    conn.execute("PRAGMA synchronous=NORMAL")
    conn.execute("PRAGMA busy_timeout=5000")
    return conn


def ensure_schema(conn: sqlite3.Connection) -> None:
    for statement in SCHEMA_SQL:
        conn.execute(statement)
    conn.commit()


def insert_task_run(conn: sqlite3.Connection, run_id: str, mode: str, args: argparse.Namespace) -> None:
    conn.execute(
        """
        INSERT OR REPLACE INTO factor_task_runs
        (id, task_type, snapshot_date, status, started_at, total_count, success_count, failed_count, skipped_count, params_json, summary_json, error_message)
        VALUES (?, 'backfill', ?, 'running', ?, 0, 0, 0, 0, ?, '{}', '')
        """,
        (run_id, args.snapshot_date or "", utc_now(), json.dumps({"mode": mode, "args": vars(args)}, ensure_ascii=False, default=str)),
    )
    conn.commit()


def finish_task_run(conn: sqlite3.Connection, run_id: str, status: str, stats: TaskStats, summary: dict[str, Any], error: str = "") -> None:
    conn.execute(
        """
        UPDATE factor_task_runs
        SET status = ?, finished_at = ?, total_count = ?, success_count = ?, failed_count = ?, skipped_count = ?, summary_json = ?, error_message = ?
        WHERE id = ?
        """,
        (status, utc_now(), stats.total, stats.success, stats.failed, stats.skipped, json.dumps(summary, ensure_ascii=False), error, run_id),
    )
    conn.commit()


def upsert_task_item(conn: sqlite3.Connection, run_id: str, item_type: str, item_key: str, status: str, error: str = "") -> None:
    conn.execute(
        """
        INSERT OR REPLACE INTO factor_task_items (run_id, item_type, item_key, status, error_message, updated_at)
        VALUES (?, ?, ?, ?, ?, ?)
        """,
        (run_id, item_type, item_key, status, error, utc_now()),
    )


def was_successful(conn: sqlite3.Connection, item_type: str, item_key: str) -> bool:
    row = conn.execute(
        "SELECT 1 FROM factor_task_items WHERE item_type = ? AND item_key = ? AND status = 'success' LIMIT 1",
        (item_type, item_key),
    ).fetchone()
    return row is not None


def chunked(items: list[Any], size: int) -> Iterable[list[Any]]:
    for start in range(0, len(items), size):
        yield items[start:start + size]


def source_order(selected: str, order: list[str]) -> list[str]:
    return order if selected == "auto" else [selected]


def log_source_failure(mode: str, item: str, exc: Exception, args: argparse.Namespace) -> None:
    log_step(f"{mode}: 数据源 {item} 失败：{exc}")
    if args.verbose:
        traceback.print_exc()


def eastmoney_secid(code: str, is_index: bool = False) -> str:
    code = normalize_code(code)
    if is_index:
        return f"1.{code}" if code.startswith("0") else f"0.{code}"
    return f"1.{code}" if infer_exchange(code) == "SSE" else f"0.{code}"


def tencent_symbol(code: str, is_index: bool = False) -> str:
    code = normalize_code(code)
    if is_index:
        return f"sh{code}" if code.startswith("0") else f"sz{code}"
    return tencent_quote_code(code)


def get_target_codes(conn: sqlite3.Connection, code: str, limit: int) -> list[str]:
    if code:
        return [normalize_code(code)]
    rows = conn.execute(
        """
        SELECT code FROM factor_securities
        WHERE is_active = 1 AND is_st = 0 AND board IN ('MAIN', 'CHINEXT')
        ORDER BY code ASC
        """
    ).fetchall()
    codes = [row[0] for row in rows]
    if not codes:
        fallback_queries = [
            "SELECT code FROM quadrant_scores WHERE exchange IN ('SSE','SZSE') AND board IN ('MAIN','CHINEXT') ORDER BY code ASC",
            "SELECT code FROM company_profiles WHERE code <> '' AND exchange IN ('SSE','SZSE') ORDER BY code ASC",
        ]
        seen: dict[str, None] = {}
        for query in fallback_queries:
            try:
                for (raw_code,) in conn.execute(query).fetchall():
                    normalized = normalize_code(raw_code)
                    if normalized and classify_board(normalized) in {"MAIN", "CHINEXT"}:
                        seen.setdefault(normalized, None)
            except sqlite3.Error:
                continue
        codes = sorted(seen.keys())
    if limit > 0:
        codes = codes[:limit]
    return codes


def load_local_code_universe(conn: sqlite3.Connection, args: argparse.Namespace, include_all_boards: bool = True) -> list[str]:
    if args.code:
        return [normalize_code(args.code)]
    codes: dict[str, None] = {}
    queries = [
        "SELECT code FROM factor_securities WHERE code <> '' ORDER BY code ASC",
        "SELECT code FROM quadrant_scores WHERE exchange IN ('SSE','SZSE') ORDER BY code ASC",
        "SELECT code FROM company_profiles WHERE exchange IN ('SSE','SZSE') ORDER BY code ASC",
    ]
    for query in queries:
        try:
            for (raw_code,) in conn.execute(query).fetchall():
                code = normalize_code(raw_code)
                if not code:
                    continue
                if not include_all_boards and classify_board(code) not in {"MAIN", "CHINEXT"}:
                    continue
                codes.setdefault(code, None)
        except sqlite3.Error:
            continue
    result = sorted(codes.keys())
    if args.limit > 0:
        result = result[:args.limit]
    return result


def build_security_payload_from_quote_records(records: list[dict[str, Any]], args: argparse.Namespace, source: str) -> SecuritiesPayload:
    rows: list[tuple[Any, ...]] = []
    metrics: list[tuple[Any, ...]] = []
    trade_date = args.snapshot_date or datetime.today().strftime("%Y-%m-%d")
    now = utc_now()
    target_code = normalize_code(args.code) if args.code else ""
    seen: set[str] = set()
    for record in records:
        code = normalize_code(record.get("code"))
        if not code or code in seen:
            continue
        if target_code and code != target_code:
            continue
        name = str(record.get("name") or "").strip()
        board = str(record.get("board") or "").strip().upper() or classify_board(code)
        exchange = str(record.get("exchange") or "").strip().upper() or infer_exchange(code)
        rows.append((code, infer_symbol(code), name, exchange, board, "", int(is_st_name(name)), 1, source, now))
        price = safe_float(record.get("price"))
        volume = safe_float(record.get("volume")) or 0.0
        amount = safe_float(record.get("amount")) or 0.0
        metrics.append((
            code,
            trade_date,
            price or 0.0,
            safe_float(record.get("market_cap")),
            safe_float(record.get("pe")),
            safe_float(record.get("pb")),
            volume,
            amount,
            safe_float(record.get("turnover_rate")),
            int((price or 0) <= 0 or volume <= 0),
            source,
            now,
        ))
        seen.add(code)
        if args.limit and len(rows) >= args.limit:
            break
    return SecuritiesPayload(rows=rows, metrics=metrics, source=source)


def fetch_securities_akshare(args: argparse.Namespace) -> list[dict[str, Any]]:
    log_step("securities: 尝试外部源 1/3 AkShare stock_zh_a_spot_em")
    ak = import_akshare()
    df = ak.stock_zh_a_spot_em()
    if df is None or df.empty:
        raise RuntimeError("stock_zh_a_spot_em 返回空数据")
    code_col = find_column(df.columns, CODE_ALIASES)
    name_col = find_column(df.columns, NAME_ALIASES)
    if not code_col or not name_col:
        raise RuntimeError("A 股列表缺少代码或名称列")
    records: list[dict[str, Any]] = []
    for _, row in df.iterrows():
        records.append({
            "code": row.get(code_col),
            "name": row.get(name_col),
            "price": row.get("最新价"),
            "volume": row.get("成交量"),
            "amount": row.get("成交额"),
            "market_cap": row.get("总市值"),
            "pe": row.get("市盈率-动态"),
            "pb": row.get("市净率"),
            "turnover_rate": row.get("换手率"),
        })
    return records


def fetch_securities_eastmoney_direct(args: argparse.Namespace) -> list[dict[str, Any]]:
    log_step("securities: 尝试外部源 2/3 东方财富 direct API")
    requests = import_requests()
    headers = {
        "User-Agent": "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 Chrome/131 Safari/537.36",
        "Referer": "https://quote.eastmoney.com/",
    }
    fields = "f12,f14,f2,f5,f6,f8,f9,f20,f23"
    records: list[dict[str, Any]] = []
    page = 1
    page_size = 200
    while True:
        params = {
            "pn": page,
            "pz": page_size,
            "po": 1,
            "np": 1,
            "ut": "bd1d9ddb04089700cf9c27f6f7426281",
            "fltt": 2,
            "invt": 2,
            "fid": "f3",
            "fs": "m:0+t:6,m:0+t:80,m:1+t:2,m:1+t:23",
            "fields": fields,
        }
        response = requests.get(EASTMONEY_CLIST_URL, params=params, headers=headers, timeout=15)
        response.raise_for_status()
        payload = response.json()
        data = payload.get("data") or {}
        diff = data.get("diff") or []
        if not diff:
            break
        for item in diff:
            records.append({
                "code": item.get("f12"),
                "name": item.get("f14"),
                "price": item.get("f2"),
                "volume": item.get("f5"),
                "amount": item.get("f6"),
                "turnover_rate": item.get("f8"),
                "pe": item.get("f9"),
                "market_cap": item.get("f20"),
                "pb": item.get("f23"),
            })
        log_step(f"securities: 东方财富 direct 已拉取 {len(records)} 条")
        total = int(data.get("total") or 0)
        if len(records) >= total or len(diff) < page_size:
            break
        if args.limit and len(records) >= args.limit:
            break
        page += 1
    if not records:
        raise RuntimeError("东方财富 direct API 返回空数据")
    return records


def tencent_quote_code(code: str) -> str:
    code = normalize_code(code)
    return f"sh{code}" if infer_exchange(code) == "SSE" else f"sz{code}"


def parse_tencent_quote_line(line: str) -> Optional[dict[str, Any]]:
    if "~" not in line:
        return None
    parts = line.split("~")
    if len(parts) < 47:
        return None
    code = normalize_code(parts[2])
    if not code:
        return None
    return {
        "code": code,
        "name": parts[1],
        "price": safe_float(parts[3]),
        "volume": safe_float(parts[36]),
        "amount": safe_scaled_float(parts[37], 1e4),
        "turnover_rate": safe_float(parts[38]),
        "pe": safe_float(parts[39]),
        "market_cap": safe_scaled_float(parts[45], 1e8),
        "pb": safe_float(parts[46]),
    }


def fetch_securities_tencent(conn: sqlite3.Connection, args: argparse.Namespace) -> list[dict[str, Any]]:
    log_step("securities: 尝试外部源 3/3 腾讯行情")
    codes = load_local_code_universe(conn, args, include_all_boards=True)
    if not codes:
        raise RuntimeError("腾讯行情需要本地 code universe，但 factor_securities/quadrant_scores/company_profiles 均为空")
    requests = import_requests()
    headers = {
        "User-Agent": "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 Chrome/131 Safari/537.36",
        "Referer": "https://stockapp.finance.qq.com/",
    }
    records: list[dict[str, Any]] = []
    batches = list(chunked(codes, 500))
    for idx, batch in enumerate(batches, start=1):
        query = ",".join(tencent_quote_code(code) for code in batch)
        response = requests.get(TENCENT_QUOTE_URL.format(codes=query), headers=headers, timeout=15)
        response.encoding = "gbk"
        for line in response.text.strip().splitlines():
            parsed = parse_tencent_quote_line(line)
            if parsed:
                records.append(parsed)
        log_progress("securities: 腾讯行情批次", idx, len(batches), args.progress_interval)
    if not records:
        raise RuntimeError("腾讯行情返回空数据")
    return records


def fetch_securities_local(conn: sqlite3.Connection, args: argparse.Namespace) -> list[dict[str, Any]]:
    log_step("securities: 使用本地兜底源 quadrant_scores + company_profiles")
    records_by_code: dict[str, dict[str, Any]] = {}
    target_code = normalize_code(args.code) if args.code else ""
    try:
        rows = conn.execute(
            """
            SELECT code, name, exchange, board
            FROM quadrant_scores
            WHERE exchange IN ('SSE','SZSE')
            ORDER BY code ASC
            """
        ).fetchall()
        for code, name, exchange, board in rows:
            normalized = normalize_code(code)
            if not normalized or (target_code and normalized != target_code):
                continue
            records_by_code[normalized] = {"code": normalized, "name": name, "exchange": exchange, "board": board}
            if args.limit and len(records_by_code) >= args.limit:
                break
    except sqlite3.Error as exc:
        log_step(f"securities: 本地 quadrant_scores 不可用：{exc}")
    if not args.limit or len(records_by_code) < args.limit:
        try:
            rows = conn.execute(
                """
                SELECT code, name, exchange, board_code
                FROM company_profiles
                WHERE code <> '' AND exchange IN ('SSE','SZSE')
                ORDER BY code ASC
                """
            ).fetchall()
            for code, name, exchange, board in rows:
                normalized = normalize_code(code)
                if not normalized or normalized in records_by_code or (target_code and normalized != target_code):
                    continue
                records_by_code[normalized] = {"code": normalized, "name": name, "exchange": exchange, "board": board or classify_board(normalized)}
                if args.limit and len(records_by_code) >= args.limit:
                    break
        except sqlite3.Error as exc:
            log_step(f"securities: 本地 company_profiles 不可用：{exc}")
    records = list(records_by_code.values())
    if not records:
        raise RuntimeError("本地 quadrant_scores/company_profiles 无可用股票池数据")
    return records


def fetch_securities_payload(conn: sqlite3.Connection, args: argparse.Namespace) -> SecuritiesPayload:
    source = args.securities_source
    source_order = SECURITIES_SOURCE_ORDER if source == "auto" else [source]
    failures: list[str] = []
    for item in source_order:
        try:
            if item == "akshare":
                records = fetch_securities_akshare(args)
                payload = build_security_payload_from_quote_records(records, args, "akshare:stock_zh_a_spot_em")
            elif item == "eastmoney":
                records = fetch_securities_eastmoney_direct(args)
                payload = build_security_payload_from_quote_records(records, args, "eastmoney:qt_clist_get")
            elif item == "tencent":
                records = fetch_securities_tencent(conn, args)
                payload = build_security_payload_from_quote_records(records, args, "tencent:qt_gtimg")
            elif item == "local":
                records = fetch_securities_local(conn, args)
                payload = build_security_payload_from_quote_records(records, args, "local:quadrant_scores+company_profiles")
            else:
                raise ValueError(f"未知 securities source: {item}")
            if payload.rows:
                log_step(f"securities: 数据源 {item} 成功，股票 {len(payload.rows)} 条，市场指标 {len(payload.metrics)} 条")
                return payload
            raise RuntimeError("数据源返回 0 条有效股票")
        except Exception as exc:  # noqa: BLE001 - source fallback needs broad isolation
            message = f"{item} failed: {exc}"
            failures.append(message)
            log_step(f"securities: 数据源 {item} 失败：{exc}")
            if args.verbose:
                traceback.print_exc()
            if source != "auto":
                break
    raise RuntimeError("securities 所有数据源均失败：" + " | ".join(failures))


def backfill_securities(conn: sqlite3.Connection, args: argparse.Namespace, run_id: str) -> TaskStats:
    log_step(f"securities: 开始补全，source={args.securities_source}, limit={args.limit or 'all'}, dry_run={not args.write}")
    payload = fetch_securities_payload(conn, args)
    stats = TaskStats(total=len(payload.rows))
    if not args.write:
        print(f"[dry-run] securities={len(payload.rows)} market_metrics={len(payload.metrics)} source={payload.source}", flush=True)
        stats.skipped = len(payload.rows)
        return stats
    conn.executemany(
        """
        INSERT OR REPLACE INTO factor_securities
        (code, symbol, name, exchange, board, listing_date, is_st, is_active, source, updated_at)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
        """,
        payload.rows,
    )
    conn.executemany(
        """
        INSERT OR REPLACE INTO factor_market_metrics
        (code, trade_date, close_price, market_cap, pe, pb, volume, amount, turnover_rate, is_suspended, source, updated_at)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
        """,
        payload.metrics,
    )
    for idx, (code, *_rest) in enumerate(payload.rows, start=1):
        upsert_task_item(conn, run_id, "security", code, "success")
        log_progress("securities: 写入进度", idx, len(payload.rows), args.progress_interval)
    conn.commit()
    stats.success = len(payload.rows)
    log_step(f"securities: 写入完成，股票 {stats.success} 条，市场指标 {len(payload.metrics)} 条")
    return stats


def parse_eastmoney_kline_row(code: str, raw: str, source: str, adjusted: str) -> Optional[tuple[Any, ...]]:
    parts = str(raw or "").split(",")
    if len(parts) < 7:
        return None
    trade_date = normalize_date(parts[0])
    if not trade_date:
        return None
    return (
        code,
        trade_date,
        safe_float(parts[1]) or 0.0,
        safe_float(parts[2]) or 0.0,
        safe_float(parts[3]) or 0.0,
        safe_float(parts[4]) or 0.0,
        safe_float(parts[5]) or 0.0,
        safe_float(parts[6]) or 0.0,
        safe_float(parts[10]) if len(parts) > 10 else None,
        adjusted,
        source,
        utc_now(),
    )


def fetch_daily_bars_akshare(code: str, start_date: str, end_date: str, args: argparse.Namespace) -> list[tuple[Any, ...]]:
    ak = import_akshare()
    df = ak.stock_zh_a_hist(symbol=code, period="daily", start_date=start_date, end_date=end_date, adjust=args.adjust)
    if df is None or df.empty:
        raise RuntimeError("AKShare 日线为空")
    rows: list[tuple[Any, ...]] = []
    now = utc_now()
    for _, row in df.iterrows():
        trade_date = normalize_date(row.get("日期"))
        if not trade_date:
            continue
        rows.append((
            code, trade_date,
            safe_float(row.get("开盘")) or 0.0,
            safe_float(row.get("收盘")) or 0.0,
            safe_float(row.get("最高")) or 0.0,
            safe_float(row.get("最低")) or 0.0,
            safe_float(row.get("成交量")) or 0.0,
            safe_float(row.get("成交额")) or 0.0,
            safe_float(row.get("换手率")),
            args.adjust,
            "akshare:stock_zh_a_hist",
            now,
        ))
    if not rows:
        raise RuntimeError("AKShare 日线无有效行")
    return rows


def fetch_daily_bars_eastmoney(code: str, start_date: str, end_date: str, args: argparse.Namespace, is_index: bool = False) -> list[tuple[Any, ...]]:
    requests = import_requests()
    fqt = "1" if args.adjust == "qfq" else ("2" if args.adjust == "hfq" else "0")
    params = {
        "secid": eastmoney_secid(code, is_index=is_index),
        "klt": 101,
        "fqt": fqt,
        "beg": start_date,
        "end": end_date,
        "fields1": "f1,f2,f3,f4,f5,f6",
        "fields2": "f51,f52,f53,f54,f55,f56,f57,f58,f59,f60,f61",
    }
    resp = requests.get(EASTMONEY_KLINE_URL, params=params, timeout=15)
    resp.raise_for_status()
    klines = ((resp.json().get("data") or {}).get("klines") or [])
    rows = [row for raw in klines if (row := parse_eastmoney_kline_row(code, raw, "eastmoney:kline", args.adjust))]
    if not rows:
        raise RuntimeError("东方财富日线为空")
    return rows


def fetch_daily_bars_tencent(code: str, start_date: str, end_date: str, args: argparse.Namespace, is_index: bool = False) -> list[tuple[Any, ...]]:
    requests = import_requests()
    symbol = tencent_symbol(code, is_index=is_index)
    start = f"{start_date[:4]}-{start_date[4:6]}-{start_date[6:]}"
    end = f"{end_date[:4]}-{end_date[4:6]}-{end_date[6:]}"
    fq = "qfq" if args.adjust == "qfq" else ""
    url = f"{TENCENT_KLINE_URL}?param={symbol},day,{start},{end},500,{fq}"
    resp = requests.get(url, timeout=15)
    resp.raise_for_status()
    data = (resp.json().get("data") or {}).get(symbol) or {}
    klines = data.get("qfqday") or data.get("day") or []
    rows: list[tuple[Any, ...]] = []
    now = utc_now()
    for item in klines:
        if len(item) < 6:
            continue
        trade_date = normalize_date(item[0])
        if not trade_date:
            continue
        rows.append((
            code, trade_date,
            safe_float(item[1]) or 0.0,
            safe_float(item[2]) or 0.0,
            safe_float(item[3]) or 0.0,
            safe_float(item[4]) or 0.0,
            safe_float(item[5]) or 0.0,
            safe_float(item[6]) if len(item) > 6 else 0.0,
            None,
            args.adjust,
            "tencent:fqkline",
            now,
        ))
    if not rows:
        raise RuntimeError("腾讯日线为空")
    return rows


def fetch_daily_bars_with_fallback(code: str, start_date: str, end_date: str, args: argparse.Namespace, is_index: bool = False) -> tuple[list[tuple[Any, ...]], str]:
    mode_label = "index-bars" if is_index else "daily-bars"
    failures: list[str] = []
    for item in source_order(args.index_bars_source if is_index else args.daily_bars_source, EXTERNAL_SOURCE_ORDER):
        try:
            log_step(f"{mode_label}: {code} 尝试数据源 {item}")
            if item == "akshare":
                if is_index:
                    rows = fetch_index_bars_akshare(code, start_date, end_date, args)
                else:
                    rows = fetch_daily_bars_akshare(code, start_date, end_date, args)
            elif item == "eastmoney":
                rows = fetch_daily_bars_eastmoney(code, start_date, end_date, args, is_index=is_index)
            elif item == "tencent":
                rows = fetch_daily_bars_tencent(code, start_date, end_date, args, is_index=is_index)
            else:
                raise ValueError(f"未知 source: {item}")
            return rows, item
        except Exception as exc:  # noqa: BLE001
            failures.append(f"{item}: {exc}")
            log_source_failure(mode_label, item, exc, args)
    raise RuntimeError("; ".join(failures))


def backfill_daily_bars(conn: sqlite3.Connection, args: argparse.Namespace, run_id: str) -> TaskStats:
    log_step(f"daily-bars: 开始补全，source={args.daily_bars_source}, limit={args.limit or 'all'}, code={args.code or 'all'}, dry_run={not args.write}")
    start_date = args.start_date or (datetime.today() - timedelta(days=args.lookback_days)).strftime("%Y%m%d")
    end_date = args.end_date or datetime.today().strftime("%Y%m%d")
    start_date = start_date.replace("-", "")
    end_date = end_date.replace("-", "")
    codes = get_target_codes(conn, args.code, args.limit)
    stats = TaskStats(total=len(codes))
    log_step(f"daily-bars: 待处理股票 {len(codes)} 只，日期 {start_date}~{end_date}")
    for idx, code in enumerate(codes, start=1):
        log_progress("daily-bars: 处理进度", idx, len(codes), args.progress_interval)
        item_key = f"daily_bar:{code}:{start_date}:{end_date}:{args.adjust}"
        if args.resume and was_successful(conn, "daily_bar", item_key):
            stats.skipped += 1
            continue
        try:
            rows, used_source = fetch_daily_bars_with_fallback(code, start_date, end_date, args)
            if args.write:
                conn.executemany(
                    """
                    INSERT OR REPLACE INTO factor_daily_bars
                    (code, trade_date, open, close, high, low, volume, amount, turnover_rate, adjusted, source, updated_at)
                    VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
                    """,
                    rows,
                )
                upsert_task_item(conn, run_id, "daily_bar", item_key, "success")
                conn.commit()
            print(f"daily-bars {code}: {len(rows)} rows source={used_source}", flush=True)
            stats.success += 1
        except Exception as exc:  # noqa: BLE001 - per-item isolation is intended
            stats.failed += 1
            if args.write:
                upsert_task_item(conn, run_id, "daily_bar", item_key, "failed", str(exc))
                conn.commit()
            print(f"daily-bars {code}: failed: {exc}", file=sys.stderr, flush=True)
        if args.sleep > 0:
            time.sleep(args.sleep)
    return stats


def index_rows_from_daily_rows(index_code: str, daily_rows: list[tuple[Any, ...]], source: str) -> list[tuple[Any, ...]]:
    sorted_rows = sorted(daily_rows, key=lambda row: row[1])
    rows: list[tuple[Any, ...]] = []
    prev_close = None
    now = utc_now()
    for row in sorted_rows:
        close = safe_float(row[3]) or 0.0
        pct_change = None
        if prev_close and prev_close > 0 and close > 0:
            pct_change = (close / prev_close - 1) * 100
        rows.append((index_code, row[1], close, pct_change, source, now))
        prev_close = close
    return rows


def fetch_index_bars_akshare(index_code: str, start_date: str, end_date: str, args: argparse.Namespace) -> list[tuple[Any, ...]]:
    ak = import_akshare()
    df = ak.index_zh_a_hist(symbol=index_code, period="daily", start_date=start_date, end_date=end_date)
    if df is None or df.empty:
        raise RuntimeError("AKShare 指数日线为空")
    daily_rows: list[tuple[Any, ...]] = []
    now = utc_now()
    for _, row in df.iterrows():
        trade_date = normalize_date(row.get("日期"))
        if not trade_date:
            continue
        daily_rows.append((index_code, trade_date, 0.0, safe_float(row.get("收盘")) or 0.0, 0.0, 0.0, 0.0, 0.0, None, args.adjust, "akshare:index_zh_a_hist", now))
    rows = index_rows_from_daily_rows(index_code, daily_rows, "akshare:index_zh_a_hist")
    if not rows:
        raise RuntimeError("AKShare 指数日线无有效行")
    return rows


def backfill_index_bars(conn: sqlite3.Connection, args: argparse.Namespace, run_id: str) -> TaskStats:
    log_step(f"index-bars: 开始补全中证全指 {args.index_code}，source={args.index_bars_source}, dry_run={not args.write}")
    start_date = args.start_date or (datetime.today() - timedelta(days=args.lookback_days)).strftime("%Y%m%d")
    end_date = args.end_date or datetime.today().strftime("%Y%m%d")
    start_date = start_date.replace("-", "")
    end_date = end_date.replace("-", "")
    index_code = args.index_code
    stats = TaskStats(total=1)
    try:
        rows, used_source = fetch_daily_bars_with_fallback(index_code, start_date, end_date, args, is_index=True)
        index_rows = rows if len(rows[0]) == 6 else index_rows_from_daily_rows(index_code, rows, f"{used_source}:index")
        log_step(f"index-bars: 拉取完成 {len(index_rows)} 条，source={used_source}")
        if args.write:
            conn.executemany(
                """
                INSERT OR REPLACE INTO factor_index_daily_bars
                (index_code, trade_date, close, pct_change, source, updated_at)
                VALUES (?, ?, ?, ?, ?, ?)
                """,
                index_rows,
            )
            upsert_task_item(conn, run_id, "index_bar", index_code, "success")
            conn.commit()
        print(f"index-bars {index_code}: {len(index_rows)} rows source={used_source}", flush=True)
        stats.success = 1
    except Exception as exc:  # noqa: BLE001
        stats.failed = 1
        if args.write:
            upsert_task_item(conn, run_id, "index_bar", index_code, "failed", str(exc))
            conn.commit()
        raise
    return stats


def frame_by_code(df: Any, value_aliases: list[str], extra_aliases: Optional[list[str]] = None) -> dict[str, dict[str, Any]]:
    if df is None or df.empty:
        return {}
    code_col = find_column(df.columns, CODE_ALIASES)
    if not code_col:
        return {}
    columns = {"value": find_column(df.columns, value_aliases)}
    if extra_aliases:
        columns["extra"] = find_column(df.columns, extra_aliases)
    result: dict[str, dict[str, Any]] = {}
    for _, row in df.iterrows():
        code = normalize_code(row.get(code_col))
        if not code:
            continue
        result[code] = {name: row.get(col) if col else None for name, col in columns.items()}
    return result


def parse_financial_frame_rows(yjbb: Any, zcfz: Any, xjll: Any, report_date: str, target_codes: set[str], source: str) -> list[tuple[Any, ...]]:
    if yjbb is None or yjbb.empty:
        return []
    code_col = find_column(yjbb.columns, CODE_ALIASES)
    if not code_col:
        return []
    revenue_col = find_column(yjbb.columns, REVENUE_ALIASES)
    revenue_yoy_col = find_column(yjbb.columns, REVENUE_YOY_ALIASES)
    profit_col = find_column(yjbb.columns, NET_PROFIT_ALIASES)
    profit_yoy_col = find_column(yjbb.columns, NET_PROFIT_YOY_ALIASES)
    report_date_col = find_column(yjbb.columns, REPORT_DATE_ALIASES)
    assets = frame_by_code(zcfz, TOTAL_ASSETS_ALIASES, TOTAL_EQUITY_ALIASES)
    cashflows = frame_by_code(xjll, OPERATING_CF_ALIASES)
    rows: list[tuple[Any, ...]] = []
    now = utc_now()
    for _, row in yjbb.iterrows():
        code = normalize_code(row.get(code_col))
        if not code or (target_codes and code not in target_codes):
            continue
        asset_row = assets.get(code, {})
        cf_row = cashflows.get(code, {})
        rows.append((
            code,
            normalize_date(report_date),
            normalize_date(row.get(report_date_col)) if report_date_col else "",
            safe_float(row.get(revenue_col)) if revenue_col else None,
            safe_float(row.get(revenue_yoy_col)) if revenue_yoy_col else None,
            safe_float(row.get(profit_col)) if profit_col else None,
            safe_float(row.get(profit_yoy_col)) if profit_yoy_col else None,
            safe_float(asset_row.get("value")),
            safe_float(asset_row.get("extra")),
            safe_float(cf_row.get("value")),
            source,
            now,
        ))
    return rows


def fetch_financials_akshare(report_dates: list[str], target_codes: set[str], args: argparse.Namespace) -> list[tuple[Any, ...]]:
    log_step("financials: 尝试数据源 AKShare")
    ak = import_akshare()
    rows_by_key: dict[tuple[str, str], tuple[Any, ...]] = {}
    for report_idx, report_date in enumerate(report_dates, start=1):
        log_progress("financials/akshare: 报告期进度", report_idx, len(report_dates), 1)
        yjbb = ak.stock_yjbb_em(date=report_date)
        zcfz = None
        xjll = None
        try:
            zcfz = ak.stock_zcfz_em(date=report_date)
        except Exception as exc:  # noqa: BLE001
            log_step(f"financials/akshare: zcfz {report_date} 失败：{exc}")
        try:
            xjll = ak.stock_xjll_em(date=report_date)
        except Exception as exc:  # noqa: BLE001
            log_step(f"financials/akshare: xjll {report_date} 失败：{exc}")
        for row in parse_financial_frame_rows(yjbb, zcfz, xjll, report_date, target_codes, "akshare:financial_reports"):
            rows_by_key[(row[0], row[1])] = row
    rows = list(rows_by_key.values())
    if not rows:
        raise RuntimeError("AKShare 财务数据为空")
    return rows


def fetch_eastmoney_datacenter(report_name: str, report_date: str, page_size: int = 500) -> Any:
    requests = import_requests()
    pd = import_pandas()
    rows: list[dict[str, Any]] = []
    page = 1
    while True:
        params = {
            "sortColumns": "SECURITY_CODE",
            "sortTypes": "1",
            "pageSize": page_size,
            "pageNumber": page,
            "reportName": report_name,
            "columns": "ALL",
            "filter": f"(REPORT_DATE='{report_date[:4]}-{report_date[4:6]}-{report_date[6:]}')",
            "source": "WEB",
            "client": "WEB",
        }
        resp = requests.get(EASTMONEY_DATACENTER_URL, params=params, timeout=15)
        resp.raise_for_status()
        result = resp.json().get("result") or {}
        data = result.get("data") or []
        if not data:
            break
        rows.extend(data)
        pages = int(result.get("pages") or 1)
        if page >= pages:
            break
        page += 1
    return pd.DataFrame(rows)


def fetch_financials_eastmoney(report_dates: list[str], target_codes: set[str], args: argparse.Namespace) -> list[tuple[Any, ...]]:
    log_step("financials: 尝试数据源 东方财富 direct API")
    rows_by_key: dict[tuple[str, str], tuple[Any, ...]] = {}
    for report_idx, report_date in enumerate(report_dates, start=1):
        log_progress("financials/eastmoney: 报告期进度", report_idx, len(report_dates), 1)
        yjbb = fetch_eastmoney_datacenter("RPT_LICO_FN_CPD", report_date)
        # 直接接口字段会随上游变化；资产负债/现金流作为增强项，失败不阻断利润表导入。
        zcfz = None
        xjll = None
        try:
            zcfz = fetch_eastmoney_datacenter("RPT_DMSK_FN_BALANCE", report_date)
        except Exception as exc:  # noqa: BLE001
            log_step(f"financials/eastmoney: balance {report_date} 失败：{exc}")
        try:
            xjll = fetch_eastmoney_datacenter("RPT_DMSK_FN_CASHFLOW", report_date)
        except Exception as exc:  # noqa: BLE001
            log_step(f"financials/eastmoney: cashflow {report_date} 失败：{exc}")
        for row in parse_financial_frame_rows(yjbb, zcfz, xjll, report_date, target_codes, "eastmoney:datacenter"):
            rows_by_key[(row[0], row[1])] = row
    rows = list(rows_by_key.values())
    if not rows:
        raise RuntimeError("东方财富 direct 财务数据为空")
    return rows


def fetch_financials_tencent(conn: sqlite3.Connection, args: argparse.Namespace) -> list[tuple[Any, ...]]:
    log_step("financials: 尝试数据源 腾讯/个股基础面兜底")
    sys.path.insert(0, str(PROJECT_ROOT / "quant"))
    from data.fundamentals import get_symbol_fundamentals  # type: ignore
    codes = get_target_codes(conn, args.code, args.limit)
    rows: list[tuple[Any, ...]] = []
    now = utc_now()
    for idx, code in enumerate(codes, start=1):
        log_progress("financials/tencent: 个股进度", idx, len(codes), args.progress_interval)
        payload = get_symbol_fundamentals(infer_symbol(code))
        items = payload.get("items") or {}
        meta = payload.get("meta") or {}
        report_period = meta.get("ttm_report_date") or meta.get("fy_report_date") or datetime.today().strftime("%Y-%m-%d")
        rows.append((
            code,
            normalize_date(report_period),
            meta.get("ttm_report_date") or meta.get("fy_report_date") or "",
            safe_float(items.get("revenue_fy")),
            None,
            safe_float(items.get("net_profit_fy")),
            safe_float(items.get("profit_growth_rate")),
            None,
            None,
            None,
            "tencent+fundamentals:fallback",
            now,
        ))
        if args.sleep > 0:
            time.sleep(args.sleep)
    if not rows:
        raise RuntimeError("腾讯基础面兜底返回空数据")
    return rows


def fetch_financial_rows_with_fallback(conn: sqlite3.Connection, args: argparse.Namespace) -> tuple[list[tuple[Any, ...]], str]:
    report_dates = build_report_date_candidates(args.report_limit)
    target_codes = set(get_target_codes(conn, args.code, args.limit))
    failures: list[str] = []
    for item in source_order(args.financials_source, EXTERNAL_SOURCE_ORDER):
        try:
            if item == "akshare":
                rows = fetch_financials_akshare(report_dates, target_codes, args)
            elif item == "eastmoney":
                rows = fetch_financials_eastmoney(report_dates, target_codes, args)
            elif item == "tencent":
                rows = fetch_financials_tencent(conn, args)
            else:
                raise ValueError(f"未知 source: {item}")
            log_step(f"financials: 数据源 {item} 成功，记录 {len(rows)} 条")
            return rows, item
        except Exception as exc:  # noqa: BLE001
            failures.append(f"{item}: {exc}")
            log_source_failure("financials", item, exc, args)
    raise RuntimeError("financials 所有数据源均失败：" + " | ".join(failures))


def backfill_financials(conn: sqlite3.Connection, args: argparse.Namespace, run_id: str) -> TaskStats:
    log_step(f"financials: 开始补全，source={args.financials_source}, report_limit={args.report_limit}, limit={args.limit or 'all'}, dry_run={not args.write}")
    rows, used_source = fetch_financial_rows_with_fallback(conn, args)
    stats = TaskStats(total=len(rows))
    log_step(f"financials: 解析完成 {len(rows)} 条结构化财务记录，source={used_source}")
    if not args.write:
        print(f"[dry-run] financial rows={len(rows)} source={used_source}", flush=True)
        stats.skipped = len(rows)
        return stats
    conn.executemany(
        """
        INSERT OR REPLACE INTO factor_financial_metrics
        (code, report_period, report_date, revenue, revenue_yoy, net_profit, net_profit_yoy, total_assets, total_equity, operating_cash_flow, source, updated_at)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
        """,
        rows,
    )
    for idx, (code, report_period, *_rest) in enumerate(rows, start=1):
        upsert_task_item(conn, run_id, "financial", f"{code}:{report_period}", "success")
        log_progress("financials: 写入进度", idx, len(rows), args.progress_interval)
    conn.commit()
    stats.success = len(rows)
    return stats


def parse_dividend_frame(code: str, df: Any, source: str) -> list[tuple[Any, ...]]:
    if df is None or df.empty:
        return []
    report_col = find_column(df.columns, REPORT_PERIOD_ALIASES)
    ex_col = find_column(df.columns, EX_DIVIDEND_DATE_ALIASES)
    per_share_col = find_column(df.columns, DIVIDEND_PER_SHARE_ALIASES)
    total_col = find_column(df.columns, TOTAL_DIVIDEND_ALIASES)
    rows: list[tuple[Any, ...]] = []
    now = utc_now()
    for _, row in df.iterrows():
        report_period = normalize_date(row.get(report_col)) if report_col else "unknown"
        ex_date = normalize_date(row.get(ex_col)) if ex_col else "unknown"
        if not report_period:
            report_period = "unknown"
        if not ex_date:
            ex_date = "unknown"
        rows.append((
            code,
            report_period,
            ex_date,
            safe_float(row.get(per_share_col)) if per_share_col else None,
            safe_float(row.get(total_col)) if total_col else None,
            source,
            now,
        ))
    return rows


def fetch_dividend_rows_akshare(code: str) -> list[tuple[Any, ...]]:
    ak = import_akshare()
    df = ak.stock_fhps_detail_em(symbol=code)
    rows = parse_dividend_frame(code, df, "akshare:stock_fhps_detail_em")
    if not rows:
        raise RuntimeError("AKShare 分红数据为空")
    return rows


def fetch_dividend_rows_eastmoney(code: str) -> list[tuple[Any, ...]]:
    requests = import_requests()
    pd = import_pandas()
    rows: list[dict[str, Any]] = []
    page = 1
    while True:
        params = {
            "sortColumns": "EX_DIVIDEND_DATE",
            "sortTypes": "-1",
            "pageSize": 100,
            "pageNumber": page,
            "reportName": "RPT_SHAREBONUS_DET",
            "columns": "ALL",
            "filter": f"(SECURITY_CODE=\"{code}\")",
            "source": "WEB",
            "client": "WEB",
        }
        resp = requests.get(EASTMONEY_DATACENTER_URL, params=params, timeout=15)
        resp.raise_for_status()
        result = resp.json().get("result") or {}
        data = result.get("data") or []
        if not data:
            break
        rows.extend(data)
        if page >= int(result.get("pages") or 1):
            break
        page += 1
    parsed = parse_dividend_frame(code, pd.DataFrame(rows), "eastmoney:RPT_SHAREBONUS_DET")
    if not parsed:
        raise RuntimeError("东方财富 direct 分红数据为空")
    return parsed


def fetch_dividend_rows_tencent(code: str) -> list[tuple[Any, ...]]:
    # 腾讯行情没有稳定的现金分红明细接口；这里通过个股基础面兜底拿股息率相关元数据，
    # 无法反推出每股现金分红时不伪造记录，直接返回失败让调用方明确知道该源不可用。
    sys.path.insert(0, str(PROJECT_ROOT / "quant"))
    from data.fundamentals import get_symbol_fundamentals  # type: ignore
    payload = get_symbol_fundamentals(infer_symbol(code))
    meta = payload.get("meta") or {}
    items = payload.get("items") or {}
    if safe_float(items.get("dividend_yield")) is None:
        raise RuntimeError("腾讯基础面未返回可用股息率")
    report_period = normalize_date(meta.get("dividend_report_date") or meta.get("fy_report_date")) or "unknown"
    return [(code, report_period, "unknown", None, None, "tencent+fundamentals:dividend_yield_only", utc_now())]


def fetch_dividend_rows_with_fallback(code: str, args: argparse.Namespace) -> tuple[list[tuple[Any, ...]], str]:
    failures: list[str] = []
    for item in source_order(args.dividends_source, EXTERNAL_SOURCE_ORDER):
        try:
            log_step(f"dividends: {code} 尝试数据源 {item}")
            if item == "akshare":
                rows = fetch_dividend_rows_akshare(code)
            elif item == "eastmoney":
                rows = fetch_dividend_rows_eastmoney(code)
            elif item == "tencent":
                rows = fetch_dividend_rows_tencent(code)
            else:
                raise ValueError(f"未知 source: {item}")
            return rows, item
        except Exception as exc:  # noqa: BLE001
            failures.append(f"{item}: {exc}")
            log_source_failure("dividends", item, exc, args)
    raise RuntimeError("; ".join(failures))


def backfill_dividends(conn: sqlite3.Connection, args: argparse.Namespace, run_id: str) -> TaskStats:
    log_step(f"dividends: 开始补全，source={args.dividends_source}, limit={args.limit or 'all'}, code={args.code or 'all'}, dry_run={not args.write}")
    codes = get_target_codes(conn, args.code, args.limit)
    stats = TaskStats(total=len(codes))
    log_step(f"dividends: 待处理股票 {len(codes)} 只")
    for idx, code in enumerate(codes, start=1):
        log_progress("dividends: 处理进度", idx, len(codes), args.progress_interval)
        item_key = f"dividend:{code}"
        if args.resume and was_successful(conn, "dividend", item_key):
            stats.skipped += 1
            continue
        try:
            rows, used_source = fetch_dividend_rows_with_fallback(code, args)
            if args.write:
                conn.executemany(
                    """
                    INSERT OR REPLACE INTO factor_dividend_records
                    (code, report_period, ex_dividend_date, cash_dividend_per_share, total_cash_dividend, source, updated_at)
                    VALUES (?, ?, ?, ?, ?, ?, ?)
                    """,
                    rows,
                )
                upsert_task_item(conn, run_id, "dividend", item_key, "success")
                conn.commit()
            print(f"dividends {code}: {len(rows)} rows source={used_source}", flush=True)
            stats.success += 1
        except Exception as exc:  # noqa: BLE001
            stats.failed += 1
            if args.write:
                upsert_task_item(conn, run_id, "dividend", item_key, "failed", str(exc))
                conn.commit()
            print(f"dividends {code}: failed: {exc}", file=sys.stderr, flush=True)
        if args.sleep > 0:
            time.sleep(args.sleep)
    return stats


def merge_stats(all_stats: list[TaskStats]) -> TaskStats:
    return TaskStats(
        total=sum(s.total for s in all_stats),
        success=sum(s.success for s in all_stats),
        failed=sum(s.failed for s in all_stats),
        skipped=sum(s.skipped for s in all_stats),
    )


def run_mode(conn: sqlite3.Connection, args: argparse.Namespace, run_id: str) -> TaskStats:
    if args.mode == "securities":
        return backfill_securities(conn, args, run_id)
    if args.mode == "daily-bars":
        return backfill_daily_bars(conn, args, run_id)
    if args.mode == "index-bars":
        return backfill_index_bars(conn, args, run_id)
    if args.mode == "financials":
        return backfill_financials(conn, args, run_id)
    if args.mode == "dividends":
        return backfill_dividends(conn, args, run_id)
    if args.mode == "all":
        modes = ["securities", "daily-bars", "index-bars", "financials", "dividends"]
        stats = []
        original = args.mode
        for mode in modes:
            args.mode = mode
            print(f"\n== backfill mode: {mode} ==", flush=True)
            stats.append(run_mode(conn, args, run_id))
        args.mode = original
        return merge_stats(stats)
    raise ValueError(f"unsupported mode: {args.mode}")


def parse_args(argv: list[str]) -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Factor Lab Phase 0 backfill")
    parser.add_argument("--db", default="", help="pumpkin.db 路径；默认自动查找 data/pumpkin.db")
    parser.add_argument("--mode", choices=["securities", "daily-bars", "index-bars", "financials", "dividends", "all"], default="all")
    parser.add_argument("--write", action="store_true", help="实际写入数据库；默认 dry-run")
    parser.add_argument("--resume", action="store_true", help="跳过已有 success task item")
    parser.add_argument("--force", action="store_true", help="保留参数，后续用于强制覆盖策略；当前 upsert 已幂等覆盖")
    parser.add_argument("--limit", type=int, default=0, help="最多处理多少只股票，0 表示不限制")
    parser.add_argument("--code", default="", help="只处理单只股票代码")
    parser.add_argument("--start-date", default="", help="日线开始日期 YYYYMMDD 或 YYYY-MM-DD")
    parser.add_argument("--end-date", default="", help="日线结束日期 YYYYMMDD 或 YYYY-MM-DD")
    parser.add_argument("--snapshot-date", default="", help="市场快照交易日 YYYY-MM-DD；默认今天")
    parser.add_argument("--lookback-days", type=int, default=DEFAULT_LOOKBACK_DAYS, help="未传 start-date 时回看自然日天数")
    parser.add_argument("--adjust", default=DEFAULT_ADJUST, choices=["", "qfq", "hfq"], help="日线复权口径，默认 qfq")
    parser.add_argument("--index-code", default=DEFAULT_INDEX_CODE, help="中证全指指数代码，默认 000985")
    parser.add_argument("--report-limit", type=int, default=8, help="最多扫描多少个报告期")
    parser.add_argument("--securities-source", choices=["auto", "akshare", "eastmoney", "tencent", "local"], default="auto", help="股票池数据源；auto=AKShare→东方财富→腾讯行情→本地兜底")
    parser.add_argument("--daily-bars-source", choices=["auto", "akshare", "eastmoney", "tencent"], default="auto", help="个股日线数据源；auto=AKShare→东方财富→腾讯")
    parser.add_argument("--index-bars-source", choices=["auto", "akshare", "eastmoney", "tencent"], default="auto", help="指数日线数据源；auto=AKShare→东方财富→腾讯")
    parser.add_argument("--financials-source", choices=["auto", "akshare", "eastmoney", "tencent"], default="auto", help="财务数据源；auto=AKShare→东方财富→腾讯基础面兜底")
    parser.add_argument("--dividends-source", choices=["auto", "akshare", "eastmoney", "tencent"], default="auto", help="分红数据源；auto=AKShare→东方财富→腾讯基础面兜底")
    parser.add_argument("--progress-interval", type=int, default=50, help="每处理多少项输出一次进度")
    parser.add_argument("--verbose", action="store_true", help="输出外部源失败的完整 traceback")
    parser.add_argument("--sleep", type=float, default=0.05, help="单股票请求间隔秒数，避免外部源限流")
    return parser.parse_args(argv)


def main(argv: list[str]) -> int:
    args = parse_args(argv)
    if args.limit < 0:
        raise ValueError("--limit 不能为负数")
    if args.lookback_days <= 0:
        raise ValueError("--lookback-days 必须大于 0")
    if args.progress_interval <= 0:
        raise ValueError("--progress-interval 必须大于 0")
    db_path = resolve_db_path(args.db)
    conn = connect_db(db_path)
    ensure_schema(conn)
    run_id = str(uuid.uuid4())
    print(f"db={db_path}", flush=True)
    print(f"run_id={run_id}", flush=True)
    print("mode=dry-run" if not args.write else "mode=write", flush=True)
    log_step(f"启动 Phase 0 backfill：mode={args.mode}")
    if args.write:
        insert_task_run(conn, run_id, args.mode, args)
    try:
        stats = run_mode(conn, args, run_id)
        status = "success" if stats.failed == 0 else ("partial" if stats.success > 0 else "failed")
        summary = {"total": stats.total, "success": stats.success, "failed": stats.failed, "skipped": stats.skipped}
        if args.write:
            finish_task_run(conn, run_id, status, stats, summary)
        print(f"summary={json.dumps(summary, ensure_ascii=False)} status={status}", flush=True)
        return 0 if status in {"success", "partial"} else 1
    except Exception as exc:  # noqa: BLE001
        if args.write:
            finish_task_run(conn, run_id, "failed", TaskStats(), {}, str(exc))
        print(f"failed: {exc}", file=sys.stderr)
        return 1
    finally:
        conn.close()


if __name__ == "__main__":
    raise SystemExit(main(sys.argv[1:]))
