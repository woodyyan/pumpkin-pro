from __future__ import annotations

import importlib.util
import sqlite3
import sys
from pathlib import Path

SCRIPT_PATH = Path(__file__).resolve().parents[1] / "scripts" / "compute_factor_lab_phase2.py"
spec = importlib.util.spec_from_file_location("compute_factor_lab_phase2", SCRIPT_PATH)
module = importlib.util.module_from_spec(spec)
assert spec.loader is not None
sys.modules[spec.name] = module
spec.loader.exec_module(module)


def setup_db(tmp_path):
    db_path = tmp_path / "factor.db"
    conn = sqlite3.connect(db_path)
    module.ensure_schema(conn)
    return conn


def seed_snapshots(conn: sqlite3.Connection):
    rows = [
        ("2026-05-08", "000001", "000001.SZ", "平安银行", "MAIN", 1000, 0, 260, 10, 100, 5, 1.0, 1.0, 0.04, 30, 20, 50, 10, 8, 15, 20, 5, 12, 0.8, "[]", "now"),
        ("2026-05-08", "000002", "000002.SZ", "万科A", "MAIN", 1000, 0, 260, 8, 80, -3, 2.0, 2.0, 0.02, 10, 5, 20, 5, 2, 8, 10, 8, 20, 1.2, "[]", "now"),
        ("2026-05-08", "000003", "000003.SZ", "样本三", "MAIN", 1000, 0, 260, 12, 120, 20, 1.5, 1.5, 0.02, 10, 5, 20, 5, 2, 8, 10, 8, 20, 1.2, "[]", "now"),
    ]
    conn.executemany(
        """
        INSERT INTO factor_snapshots
        (snapshot_date, code, symbol, name, board, listing_age_days, is_new_stock, available_trading_days,
         close_price, market_cap, pe, pb, ps, dividend_yield, earning_growth, revenue_growth,
         performance_1y, performance_since_listing, momentum_1m, roe, operating_cf_margin, asset_to_equity,
         volatility_1m, beta_1y, data_quality_flags, created_at)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
        """,
        rows,
    )
    conn.execute("INSERT INTO factor_security_industries (code, raw_industry_name, industry_name, industry_source, updated_at) VALUES ('000001','银行Ⅰ','银行','test','now')")
    conn.commit()


def test_metric_rank_scores_use_competition_rank_and_negative_replacement(tmp_path):
    conn = setup_db(tmp_path)
    try:
        seed_snapshots(conn)
        rows = module.load_snapshot_rows(conn, "2026-05-08")
        spec = next(item for item in module.METRICS if item.key == "pe")
        scores = module.compute_metric_rank_scores(rows, spec)
        assert round(scores["000001"], 4) == 100.0
        assert round(scores["000002"], 4) == round(2 / 3 * 100, 4)
        assert round(scores["000003"], 4) == round(2 / 3 * 100, 4)

        dividend_spec = next(item for item in module.METRICS if item.key == "dividend_yield")
        dividend_scores = module.compute_metric_rank_scores(rows, dividend_spec)
        assert dividend_scores["000001"] == 100.0
        assert round(dividend_scores["000002"], 4) == round(2 / 3 * 100, 4)
        assert round(dividend_scores["000003"], 4) == round(2 / 3 * 100, 4)
    finally:
        conn.close()


def test_compute_scores_writes_rank_and_factor_scores(tmp_path):
    conn = setup_db(tmp_path)
    try:
        seed_snapshots(conn)
        rank_rows, factor_rows, coverage = module.compute_scores(conn, "2026-05-08", 1000)
        assert len(rank_rows) == 3
        assert len(factor_rows) == 3
        assert coverage["pe_rank_score"] == 3
        assert coverage["value_score"] == 3
        module.write_scores(conn, "2026-05-08", rank_rows, factor_rows)
        rank = conn.execute("SELECT pe_rank_score FROM factor_rank_scores WHERE code = '000001'").fetchone()
        assert rank == (100.0,)
        score = conn.execute("SELECT industry, value_score, dividend_yield_score FROM factor_scores WHERE code = '000001'").fetchone()
        assert score[0] == "银行"
        assert score[1] is not None
        assert score[2] == 100.0
    finally:
        conn.close()


def test_factor_score_normalizes_missing_component_weights():
    rank_row = {"pe_rank_score": 100.0, "pb_rank_score": None, "ps_rank_score": 50.0}
    factor = next(item for item in module.FACTORS if item.key == "value")
    score = module.weighted_score(rank_row, factor)
    assert round(score, 4) == round((100 * 0.4 + 50 * 0.2) / 0.6, 4)
