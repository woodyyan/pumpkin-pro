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
        assert "factor_security_industries" in tables
        assert "factor_rank_scores" in tables
        assert "factor_scores" in tables
        dividend_columns = module.table_columns(conn, "factor_dividend_records")
        financial_columns = module.table_columns(conn, "factor_financial_metrics")
        snapshot_columns = module.table_columns(conn, "factor_snapshots")
        rank_score_columns = module.table_columns(conn, "factor_rank_scores")
        assert "dividend_yield" in dividend_columns
        assert "dividend_yield_source" in dividend_columns
        assert "raw_plan" in dividend_columns
        assert "capex" in financial_columns
        assert "fcf_margin" in snapshot_columns
        assert "fcf_margin_rank_score" in rank_score_columns
    finally:
        conn.close()


def test_build_security_payload_from_quote_records_applies_limit_and_metrics():
    args = module.parse_args(["--mode", "securities", "--limit", "1", "--snapshot-date", "2026-05-08"])
    payload = module.build_security_payload_from_quote_records(
        [
            {"code": "1", "name": "平安银行", "price": 11.2, "volume": 100, "amount": 1000, "market_cap": 10_000, "pe": 6.5, "pb": 0.8, "turnover_rate": 0.5, "industry": "银行Ⅰ"},
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
    assert payload.industries[0][0] == "000001"
    assert payload.industries[0][2] == "银行"
    assert payload.source == "test-source"


def test_validate_security_payload_size_rejects_truncated_full_universe():
    args = module.parse_args(["--mode", "securities"])
    payload = module.SecuritiesPayload(rows=[("000001",)], metrics=[], industries=[], source="test")
    try:
        module.validate_security_payload_size(payload, args, "eastmoney")
    except RuntimeError as exc:
        assert "低于全量股票池最小阈值" in str(exc)
    else:
        raise AssertionError("expected truncated full-universe payload to fail")


def test_validate_security_payload_size_allows_debug_scope():
    args = module.parse_args(["--mode", "securities", "--limit", "10"])
    payload = module.SecuritiesPayload(rows=[("000001",)], metrics=[], industries=[], source="test")
    module.validate_security_payload_size(payload, args, "eastmoney")


def test_request_with_retry_retries_then_succeeds(monkeypatch):
    class FakeResponse:
        def __init__(self, payload):
            self._payload = payload

        def raise_for_status(self):
            return None

        def json(self):
            return self._payload

    attempts = {"count": 0}

    class FakeRequests:
        def get(self, url, params=None, headers=None, timeout=15):
            attempts["count"] += 1
            if attempts["count"] < 3:
                raise RuntimeError("temporary network failure")
            return FakeResponse({"ok": True})

    monkeypatch.setattr(module, "import_requests", lambda: FakeRequests())
    monkeypatch.setattr(module.time, "sleep", lambda _: None)
    response = module.request_with_retry("https://example.com", retries=3, backoff_seconds=0.01)
    assert response.json() == {"ok": True}
    assert attempts["count"] == 3


def test_fetch_industry_rows_only_uses_akshare(monkeypatch, tmp_path):
    db_path = tmp_path / "factor.db"
    conn = sqlite3.connect(db_path)
    try:
        module.ensure_schema(conn)
        conn.execute("INSERT INTO factor_securities (code, symbol, name, exchange, board, is_st, is_active, source, updated_at) VALUES ('000001','000001.SZ','平安银行','SZSE','MAIN',0,1,'test','now')")
        conn.commit()
        args = module.parse_args(["--mode", "industries"])
        called = {"akshare": 0}

        def fake_fetch_industry_rows_akshare(inner_conn, inner_args):
            called["akshare"] += 1
            return [(
                "000001", "银行", "银行", "akshare:stock_board_industry_cons_em", "now"
            )], "akshare:stock_board_industry_cons_em"

        monkeypatch.setattr(module, "fetch_industry_rows_akshare", fake_fetch_industry_rows_akshare)
        rows, source = module.fetch_industry_rows(conn, args)
        assert called["akshare"] == 1
        assert source == "akshare:stock_board_industry_cons_em"
        assert rows[0][0] == "000001"
    finally:
        conn.close()


def test_fetch_securities_local_prefers_quadrant_then_company_profiles(tmp_path):
    db_path = tmp_path / "factor.db"
    conn = sqlite3.connect(db_path)
    try:
        module.ensure_schema(conn)
        conn.execute("CREATE TABLE quadrant_scores (code TEXT, name TEXT, exchange TEXT, board TEXT)")
        conn.execute("INSERT INTO quadrant_scores VALUES ('300001', '特锐德', 'SZSE', 'CHINEXT')")
        conn.execute("INSERT INTO company_profiles (code, name, exchange, board_code, created_at, updated_at) VALUES ('600519', '贵州茅台', 'SSE', 'MAIN', 'now', 'now')")
        conn.commit()
        args = module.parse_args(["--mode", "securities", "--securities-source", "local", "--limit", "2"])
        records = module.fetch_securities_local(conn, args)
        assert [item["code"] for item in records] == ["300001", "600519"]
        payload = module.fetch_securities_payload(conn, args)
        assert len(payload.rows) == 2
        assert payload.source == "local:quadrant_scores+company_profiles"
    finally:
        conn.close()


def test_build_industry_refresh_payload_maps_to_company_profiles(tmp_path):
    db_path = tmp_path / "factor.db"
    conn = sqlite3.connect(db_path)
    try:
        module.ensure_schema(conn)
        conn.execute("INSERT INTO factor_securities (code, symbol, name, exchange, board, is_st, is_active, source, updated_at) VALUES ('000001','000001.SZ','平安银行','SZSE','MAIN',0,1,'test','now')")
        conn.execute("INSERT INTO factor_security_industries (code, raw_industry_name, industry_name, industry_source, updated_at) VALUES ('000001','白酒Ⅱ','白酒','eastmoney:qt_clist_get','now')")
        conn.commit()
        args = module.parse_args(["--mode", "industries"])
        payload = module.build_industry_refresh_payload(conn, args)
        assert payload.total == 1
        assert payload.failed == 0
        assert payload.profiles[0][0] == "000001.SZ"
        assert payload.profiles[0][8] == "food_beverage"
        assert payload.profiles[0][9] == "食品饮料"
        assert payload.profiles[0][11] == "sw_l1"
        assert any(row[0] == "eastmoney" and row[1] == "白酒" and row[3] == "食品饮料" for row in payload.mappings)
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


def test_parse_dividend_helpers_extract_yield_and_cash_per_share():
    assert module.normalize_dividend_yield_value("2.5%") == 0.025
    assert module.normalize_dividend_yield_value("0.018") == 0.018
    assert module.parse_cash_dividend_per_share("10派2.36元(含税)") == 0.236
    assert module.parse_cash_dividend_per_share("10转4.00派1.20元") == 0.12


def test_parse_dividend_frame_maps_akshare_fields():
    pd = __import__("pandas")
    df = pd.DataFrame([
        {
            "报告期": "2025-12-31",
            "除权除息日": "2026-06-01",
            "现金分红-现金分红比例": "2.36",
            "现金分红-现金分红比例描述": "10派2.36元(含税)",
            "现金分红-股息率": "0.020397579948",
        }
    ])
    rows = module.parse_dividend_frame("000001", df, "akshare:test")
    assert rows[0][3] == 0.236
    assert rows[0][5] == 0.020397579948
    assert rows[0][6] == "akshare:test:现金分红-股息率"
    assert rows[0][7] == "10派2.36元(含税)"


def test_parse_dividend_frame_maps_eastmoney_fields():
    pd = __import__("pandas")
    df = pd.DataFrame([
        {
            "REPORT_DATE": "2025-12-31 00:00:00",
            "EX_DIVIDEND_DATE": "2026-06-01 00:00:00",
            "PRETAX_BONUS_RMB": "17.5",
            "IMPL_PLAN_PROFILE": "10派17.50元(含税)",
            "DIVIDENT_RATIO": "0.032552083333",
        }
    ])
    rows = module.parse_dividend_frame("601318", df, "eastmoney:test")
    assert rows[0][3] == 1.75
    assert rows[0][5] == 0.032552083333
    assert rows[0][6] == "eastmoney:test:DIVIDENT_RATIO"
    assert rows[0][7] == "10派17.50元(含税)"


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


def test_normalize_capex_value_keeps_cash_outflow_non_negative():
    assert module.normalize_capex_value(None) is None
    assert module.normalize_capex_value(123.0) == 123.0
    assert module.normalize_capex_value(-123.0) == 123.0
    assert module.normalize_capex_value("-456") == 456.0


def test_parse_financial_frame_rows_maps_eastmoney_cashflow_fields():
    pd = __import__("pandas")
    yjbb = pd.DataFrame([
        {
            "SECURITY_CODE": "000001",
            "REPORT_DATE": "2026-03-31 00:00:00",
            "营业收入": "1000000",
        }
    ])
    xjll = pd.DataFrame([
        {
            "SECURITY_CODE": "000001",
            "NETCASH_OPERATE": "200000",
            "CONSTRUCT_LONG_ASSET": "50000",
        }
    ])

    rows = module.parse_financial_frame_rows(yjbb, None, xjll, "20260331", {"000001"}, "eastmoney:test")

    assert len(rows) == 1
    assert rows[0][3] == 1000000.0
    assert rows[0][9] == 200000.0
    assert rows[0][10] == 50000.0
