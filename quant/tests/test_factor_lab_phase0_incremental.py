from __future__ import annotations

import importlib.util
import sqlite3
import sys
from pathlib import Path

SCRIPT_PATH = Path(__file__).resolve().parents[1] / "scripts" / "update_factor_lab_phase0_incremental.py"
spec = importlib.util.spec_from_file_location("update_factor_lab_phase0_incremental", SCRIPT_PATH)
module = importlib.util.module_from_spec(spec)
assert spec.loader is not None
sys.modules[spec.name] = module
spec.loader.exec_module(module)


def test_split_csv_trims_empty_items():
    assert module.split_csv("securities, daily-bars,,financials ") == ["securities", "daily-bars", "financials"]


def test_buffered_start_date_uses_yyyymmdd_format():
    assert module.buffered_start_date("2026-05-20", 7) == "20260513"
    assert module.buffered_start_date("", 7) == ""


def test_latest_date_returns_empty_for_missing_table(tmp_path):
    conn = sqlite3.connect(tmp_path / "factor.db")
    try:
        assert module.latest_date(conn, "missing_table", "trade_date") == ""
    finally:
        conn.close()


def test_build_daily_bars_command_uses_latest_date_buffer(tmp_path):
    db_path = tmp_path / "factor.db"
    conn = sqlite3.connect(db_path)
    try:
        conn.execute("CREATE TABLE factor_securities (code TEXT, is_active INTEGER, is_st INTEGER, board TEXT)")
        conn.execute("CREATE TABLE factor_daily_bars (code TEXT, trade_date TEXT)")
        conn.execute("CREATE TABLE factor_market_metrics (code TEXT, trade_date TEXT)")
        conn.execute("INSERT INTO factor_securities VALUES ('000001',1,0,'MAIN'),('000002',1,0,'MAIN')")
        conn.execute("INSERT INTO factor_daily_bars VALUES ('000001', '2026-05-20')")
        conn.execute("INSERT INTO factor_market_metrics VALUES ('000001', '2026-05-20'),('000002','2026-05-20')")
        args = module.parse_args(["--db", str(db_path), "--write", "--buffer-days", "5", "--progress-interval", "123", "--item-progress-interval", "1", "--daily-bars-source", "tencent"])
        args.db_path = db_path
        args.temp_files = []
        cmd = module.build_mode_command(args, "daily-bars", conn)
        assert "--write" in cmd
        assert "--start-date" in cmd
        assert cmd[cmd.index("--start-date") + 1] == "20260515"
        assert cmd[cmd.index("--progress-interval") + 1] == "1"
        assert cmd[cmd.index("--daily-bars-source") + 1] == "tencent"
        code_list = cmd[cmd.index("--code-list-file") + 1]
        assert Path(code_list).read_text().strip() == "000002"
    finally:
        conn.close()


def test_build_index_bars_command_falls_back_to_lookback(tmp_path):
    db_path = tmp_path / "factor.db"
    conn = sqlite3.connect(db_path)
    try:
        args = module.parse_args(["--db", str(db_path), "--lookback-days", "600"])
        args.db_path = db_path
        args.temp_files = []
        cmd = module.build_mode_command(args, "index-bars", conn)
        assert "--lookback-days" in cmd
        assert cmd[cmd.index("--lookback-days") + 1] == "600"
    finally:
        conn.close()


def test_build_financial_and_dividend_commands_use_small_report_limits(tmp_path):
    db_path = tmp_path / "factor.db"
    conn = sqlite3.connect(db_path)
    try:
        conn.execute("CREATE TABLE factor_securities (code TEXT, is_active INTEGER, is_st INTEGER, board TEXT)")
        conn.execute("CREATE TABLE factor_financial_metrics (code TEXT, report_period TEXT)")
        conn.execute("CREATE TABLE factor_dividend_records (code TEXT, updated_at TEXT)")
        conn.execute("INSERT INTO factor_securities VALUES ('000001',1,0,'MAIN')")
        args = module.parse_args(["--db", str(db_path), "--financial-report-limit", "2", "--dividend-report-limit", "3"])
        args.db_path = db_path
        args.temp_files = []
        financial_cmd = module.build_mode_command(args, "financials", conn)
        dividend_cmd = module.build_mode_command(args, "dividends", conn)
        assert financial_cmd[financial_cmd.index("--report-limit") + 1] == "2"
        assert dividend_cmd[dividend_cmd.index("--report-limit") + 1] == "3"
    finally:
        conn.close()
