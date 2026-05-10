from __future__ import annotations

import importlib.util
import sqlite3
import sys
from pathlib import Path

SCRIPT_PATH = Path(__file__).resolve().parents[1] / "scripts" / "compute_factor_lab_phase1.py"
spec = importlib.util.spec_from_file_location("compute_factor_lab_phase1", SCRIPT_PATH)
module = importlib.util.module_from_spec(spec)
assert spec.loader is not None
sys.modules[spec.name] = module
spec.loader.exec_module(module)


def make_security(code="000001", listing_date="2025-01-01"):
    return module.UniverseSecurity(code=code, symbol=f"{code}.SZ", name="样本", board="MAIN", listing_date=listing_date)


def make_bars(count=260, start_close=10.0):
    bars = []
    close = start_close
    for idx in range(count):
        close += 0.1
        bars.append(module.DailyBar(trade_date=f"2025-01-{idx + 1:02d}" if idx < 28 else f"2025-02-{idx - 27:02d}", close=close, volume=1000, amount=close * 1000))
    bars[-1].trade_date = "2026-05-08"
    return bars


def test_beta_against_index_returns_none_when_benchmark_variance_is_zero():
    stock_returns = [(f"d{i}", i / 10000) for i in range(200)]
    index_returns = {f"d{i}": 0.0 for i in range(200)}
    beta, samples = module.beta_against_index(stock_returns, index_returns, min_samples=10)
    assert samples == 200
    assert beta is None


def test_beta_against_index_with_sufficient_samples():
    stock_returns = [(f"d{i}", i / 10000) for i in range(200)]
    index_returns = {f"d{i}": i / 20000 for i in range(200)}
    beta, samples = module.beta_against_index(stock_returns, index_returns, min_samples=10)
    assert samples == 200
    assert round(beta, 4) == 2.0


def test_compute_snapshot_for_security_full_metrics():
    bars = make_bars(260)
    returns = module.daily_returns(bars)
    index_returns = {date: ret / 2 for date, ret in returns}
    market = module.MarketMetric(
        close_price=36.0,
        market_cap=360_000_000.0,
        pe=12.0,
        pb=1.5,
        volume=1000,
        amount=36000,
        turnover_rate=1.2,
        is_suspended=False,
        trade_date="2026-05-08",
    )
    financial = module.FinancialMetric(
        report_period="2026-03-31",
        revenue=120_000_000.0,
        revenue_yoy=20.0,
        net_profit=12_000_000.0,
        net_profit_yoy=30.0,
        total_assets=200_000_000.0,
        total_equity=100_000_000.0,
        operating_cash_flow=24_000_000.0,
    )
    dividend = module.DividendRecord(
        report_period="2025-12-31",
        ex_dividend_date="2026-05-01",
        cash_dividend_per_share=0.5,
        total_cash_dividend=7_200_000.0,
    )
    result, reason = module.compute_snapshot_for_security(make_security(), market, bars, financial, dividend, index_returns, "2026-05-08")
    assert reason == "included"
    assert result is not None
    row = result.row
    assert row[9] == 360_000_000.0
    assert row[12] == 3.0  # PS
    assert row[13] == 0.02  # dividend_yield
    assert row[19] == 12.0  # ROE %
    assert row[20] == 20.0  # operating_cf_margin %
    assert row[21] == 2.0   # asset_to_equity
    assert row[23] is not None


def test_compute_snapshot_excludes_suspended_or_missing_date():
    bars = make_bars(30)
    market = module.MarketMetric(10, None, None, None, 0, 0, None, True, "2026-05-08")
    result, reason = module.compute_snapshot_for_security(make_security(), market, bars, None, None, {}, "2026-05-08")
    assert result is None
    assert reason == "suspended_market_metric"

    bars[-1].trade_date = "2026-05-07"
    result, reason = module.compute_snapshot_for_security(make_security(), None, bars, None, None, {}, "2026-05-08")
    assert result is None
    assert reason == "no_snapshot_date_bar"


def test_compute_snapshots_reads_local_tables(tmp_path):
    db_path = tmp_path / "factor.db"
    conn = sqlite3.connect(db_path)
    try:
        from backfill_factor_lab_phase0 import ensure_schema
        ensure_schema(conn)
        conn.execute("INSERT INTO factor_securities (code, symbol, name, exchange, board, listing_date, is_st, is_active, source, updated_at) VALUES ('000001','000001.SZ','平安银行','SZSE','MAIN','2020-01-01',0,1,'test','now')")
        conn.execute("INSERT INTO factor_market_metrics (code, trade_date, close_price, market_cap, pe, pb, volume, amount, is_suspended, source, updated_at) VALUES ('000001','2026-05-08',12,120000000,8,1,1000,12000,0,'test','now')")
        for idx in range(30):
            date = f"2026-04-{idx + 1:02d}" if idx < 30 else "2026-05-08"
            if idx == 29:
                date = "2026-05-08"
            conn.execute("INSERT INTO factor_daily_bars (code, trade_date, open, close, high, low, volume, amount, adjusted, source, updated_at) VALUES ('000001',?,?,?,?,?,?,?,'qfq','test','now')", (date, 10 + idx, 10 + idx, 10 + idx, 10 + idx, 1000, 10000))
            conn.execute("INSERT INTO factor_index_daily_bars (index_code, trade_date, close, pct_change, source, updated_at) VALUES ('000985',?,?,1,'test','now')", (date, 100 + idx))
        conn.execute("INSERT INTO factor_financial_metrics (code, report_period, revenue, revenue_yoy, net_profit, net_profit_yoy, total_assets, total_equity, operating_cash_flow, source, updated_at) VALUES ('000001','2026-03-31',1000000,10,100000,20,2000000,1000000,200000,'test','now')")
        conn.commit()
        args = module.parse_args(["--snapshot-date", "2026-05-08", "--limit", "1", "--progress-interval", "1"])
        rows, coverage = module.compute_snapshots(conn, args, "run-test")
        assert len(rows) == 1
        assert coverage.included == 1
        assert coverage.metrics["pe"] == 1
        assert coverage.excluded == {}
    finally:
        conn.close()
