from __future__ import annotations

import importlib.util
import sqlite3
import sys
from pathlib import Path

SCRIPT_PATH = Path(__file__).resolve().parents[1] / "scripts" / "backfill_factor_lab_phase0.py"
spec = importlib.util.spec_from_file_location("backfill_factor_lab_phase0", SCRIPT_PATH)
module = importlib.util.module_from_spec(spec)
assert spec.loader is not None
sys.modules[spec.name] = module
spec.loader.exec_module(module)


def test_classify_board_and_symbol_helpers():
    assert module.classify_board("688001") == "STAR"
    assert module.classify_board("300001") == "CHINEXT"
    assert module.classify_board("920001") == "BJ"
    assert module.classify_board("600519") == "MAIN"
    assert module.infer_symbol("600519") == "600519.SH"
    assert module.infer_symbol("1") == "000001.SZ"
    assert module.is_st_name("*ST 样本") is True
    assert module.is_st_name("平安银行") is False


def test_safe_float_and_date_normalization():
    assert module.safe_float("1.23") == 1.23
    assert module.safe_float("-") is None
    assert module.safe_float("not-a-number") is None
    assert module.safe_float(float("nan")) is None
    assert module.safe_scaled_float("2", 1e8) == 200000000.0
    assert module.normalize_date("2026/05/08") == "2026-05-08"
    assert module.normalize_date("") == ""


def test_ensure_schema_creates_phase0_tables(tmp_path):
    db_path = tmp_path / "factor.db"
    conn = sqlite3.connect(db_path)
    try:
        module.ensure_schema(conn)
        tables = {
            row[0]
            for row in conn.execute("SELECT name FROM sqlite_master WHERE type = 'table'").fetchall()
        }
        assert "factor_securities" in tables
        assert "factor_daily_bars" in tables
        assert "factor_index_daily_bars" in tables
        assert "factor_market_metrics" in tables
        assert "factor_financial_metrics" in tables
        assert "factor_dividend_records" in tables
        assert "factor_snapshots" in tables
        assert "factor_task_runs" in tables
        assert "factor_task_items" in tables
    finally:
        conn.close()


def test_build_security_payload_from_quote_records_applies_limit_and_metrics():
    args = module.parse_args(["--mode", "securities", "--limit", "1", "--snapshot-date", "2026-05-08"])
    payload = module.build_security_payload_from_quote_records(
        [
            {"code": "1", "name": "平安银行", "price": 11.2, "volume": 100, "amount": 1000, "market_cap": 10_000, "pe": 6.5, "pb": 0.8, "turnover_rate": 0.5},
            {"code": "600519", "name": "贵州茅台", "price": 1500},
        ],
        args,
        "test-source",
    )
    assert len(payload.rows) == 1
    assert len(payload.metrics) == 1
    assert payload.rows[0][0] == "000001"
    assert payload.rows[0][1] == "000001.SZ"
    assert payload.rows[0][4] == "MAIN"
    assert payload.metrics[0][1] == "2026-05-08"
    assert payload.metrics[0][3] == 10_000
    assert payload.source == "test-source"


def test_fetch_securities_local_prefers_quadrant_then_company_profiles(tmp_path):
    db_path = tmp_path / "factor.db"
    conn = sqlite3.connect(db_path)
    try:
        module.ensure_schema(conn)
        conn.execute("CREATE TABLE quadrant_scores (code TEXT, name TEXT, exchange TEXT, board TEXT)")
        conn.execute("CREATE TABLE company_profiles (code TEXT, name TEXT, exchange TEXT, board_code TEXT)")
        conn.execute("INSERT INTO quadrant_scores VALUES ('300001', '特锐德', 'SZSE', 'CHINEXT')")
        conn.execute("INSERT INTO company_profiles VALUES ('600519', '贵州茅台', 'SSE', 'MAIN')")
        conn.commit()
        args = module.parse_args(["--mode", "securities", "--securities-source", "local", "--limit", "2"])
        records = module.fetch_securities_local(conn, args)
        assert [item["code"] for item in records] == ["300001", "600519"]
        payload = module.fetch_securities_payload(conn, args)
        assert len(payload.rows) == 2
        assert payload.source == "local:quadrant_scores+company_profiles"
    finally:
        conn.close()


def test_parse_tencent_quote_line_extracts_market_metrics():
    parts = [""] * 50
    parts[1] = "平安银行"
    parts[2] = "000001"
    parts[3] = "11.20"
    parts[36] = "1000"
    parts[37] = "20"
    parts[38] = "0.5"
    parts[39] = "6.5"
    parts[45] = "2200"
    parts[46] = "0.8"
    line = "~".join(parts)
    parsed = module.parse_tencent_quote_line(line)
    assert parsed["code"] == "000001"
    assert parsed["amount"] == 200000.0
    assert parsed["market_cap"] == 220000000000.0
    assert parsed["pb"] == 0.8


def test_task_run_lifecycle(tmp_path):
    db_path = tmp_path / "factor.db"
    conn = sqlite3.connect(db_path)
    try:
        module.ensure_schema(conn)
        args = module.parse_args(["--mode", "securities", "--limit", "1"])
        module.insert_task_run(conn, "run-1", "securities", args)
        module.upsert_task_item(conn, "run-1", "security", "000001", "success")
        module.finish_task_run(
            conn,
            "run-1",
            "success",
            module.TaskStats(total=1, success=1),
            {"ok": True},
        )
        run = conn.execute("SELECT status, total_count, success_count FROM factor_task_runs WHERE id = 'run-1'").fetchone()
        assert run == ("success", 1, 1)
        item = conn.execute("SELECT status FROM factor_task_items WHERE run_id = 'run-1' AND item_key = '000001'").fetchone()
        assert item == ("success",)
        assert module.was_successful(conn, "security", "000001") is True
    finally:
        conn.close()


def test_parse_eastmoney_kline_row_extracts_daily_bar():
    row = module.parse_eastmoney_kline_row(
        "000001",
        "2026-05-08,10.1,10.5,10.8,9.9,12345,67890,8.1,2.3,0.2,1.5",
        "eastmoney:kline",
        "qfq",
    )
    assert row[0] == "000001"
    assert row[1] == "2026-05-08"
    assert row[3] == 10.5
    assert row[7] == 67890
    assert row[8] == 1.5
    assert row[9] == "qfq"


def test_index_rows_from_daily_rows_calculates_pct_change():
    daily_rows = [
        ("000985", "2026-05-07", 0, 100.0, 0, 0, 0, 0, None, "qfq", "test", "now"),
        ("000985", "2026-05-08", 0, 110.0, 0, 0, 0, 0, None, "qfq", "test", "now"),
    ]
    rows = module.index_rows_from_daily_rows("000985", daily_rows, "test:index")
    assert rows[0][3] is None
    assert round(rows[1][3], 4) == 10.0
    assert rows[1][4] == "test:index"


def test_daily_bars_fallback_uses_second_source(monkeypatch):
    args = module.parse_args(["--mode", "daily-bars", "--daily-bars-source", "auto"])

    def fail_akshare(code, start_date, end_date, args):
        raise RuntimeError("akshare down")

    def ok_eastmoney(code, start_date, end_date, args, is_index=False):
        return [(code, "2026-05-08", 1, 2, 3, 4, 5, 6, None, "qfq", "eastmoney:kline", "now")]

    monkeypatch.setattr(module, "fetch_daily_bars_akshare", fail_akshare)
    monkeypatch.setattr(module, "fetch_daily_bars_eastmoney", ok_eastmoney)
    rows, source = module.fetch_daily_bars_with_fallback("000001", "20260501", "20260508", args)
    assert source == "eastmoney"
    assert rows[0][0] == "000001"


def test_source_args_are_available_for_all_modes():
    args = module.parse_args([
        "--mode", "all",
        "--securities-source", "tencent",
        "--daily-bars-source", "eastmoney",
        "--index-bars-source", "tencent",
        "--financials-source", "akshare",
        "--dividends-source", "eastmoney",
        "--progress-interval", "3",
        "--verbose",
    ])
    assert args.securities_source == "tencent"
    assert args.daily_bars_source == "eastmoney"
    assert args.index_bars_source == "tencent"
    assert args.financials_source == "akshare"
    assert args.dividends_source == "eastmoney"
    assert args.progress_interval == 3
    assert args.verbose is True
