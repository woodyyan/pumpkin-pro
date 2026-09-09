# -*- coding: utf-8 -*-
"""C6 行业 ETF RS 轮动回放引擎测试。

覆盖：
- 正常路径：信号 → T+1 收盘成交 → T+2 起吃收益的全链路
- 边界：上市前掩蔽、持仓缺价、空篮子清仓、样本不足指标降级
- 异常：空宇宙、重复代码、空基准、非法参数
- 核心回归：同日收益（未来函数）必须不存在
"""
from __future__ import annotations

import sys
import unittest
from datetime import date, timedelta
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))

from strategy_portfolios.c6_rs_rotation import (  # noqa: E402
    C6_BENCHMARK,
    C6_UNIVERSE,
    DEFAULT_PARAMS,
    compute_metrics,
    run_c6_replay,
)


def make_dates(n: int, start: date = date(2026, 1, 5)) -> list:
    """生成 n 个连续自然日字符串（引擎只依赖可排序日期轴）。"""
    return [(start + timedelta(days=i)).isoformat() for i in range(n)]


def make_asset(code: str, closes: list, dates: list, name: str = "", list_date: str = "") -> dict:
    bars = [{"date": d, "close": c} for d, c in zip(dates, closes) if c is not None]
    return {"code": code, "name": name or code, "list_date": list_date, "bars": bars}


def make_benchmark(closes: list, dates: list) -> dict:
    return {"code": "510300", "name": "沪深300ETF",
            "bars": [{"date": d, "close": c} for d, c in zip(dates, closes)]}


FAST_PARAMS = {
    "top_n": 1, "rebalance_interval": 1, "lookback": 1,
    "use_timing": False, "use_stop": False,
    "cost": 0.0, "risk_free": 0.0,
}


class TestNoSameDayReturn(unittest.TestCase):
    """未来函数回归：T 日信号 → T+1 收盘成交 → T+2 起吃收益。"""

    def setUp(self):
        self.dates = make_dates(6)
        # B 每日 +10%，d4 单日暴涨；A 全程走平。
        self.assets = [
            make_asset("AAA", [100, 100, 100, 100, 100, 100], self.dates, "A"),
            make_asset("BBB", [100, 110, 121, 133.1, 300, 300], self.dates, "B"),
        ]
        self.bench = make_benchmark([100] * 6, self.dates)

    def test_signal_executes_next_day_close_and_earns_day_after(self):
        result = run_c6_replay(self.assets, self.bench, FAST_PARAMS)
        daily = {row["date"]: row for row in result["daily"]}

        # d1 收盘出信号（B 日收益 +10%），d2 收盘才成交：
        # d2 当天组合仍为空仓，净值必须保持 1.0（旧引擎此处会错误吃到 +10%）。
        self.assertAlmostEqual(daily[self.dates[2]]["nav"], 1.0, places=6)

        trades = result["trades"]
        self.assertEqual(len(trades), 1)
        self.assertEqual(trades[0]["side"], "BUY")
        self.assertEqual(trades[0]["code"], "BBB")
        self.assertEqual(trades[0]["signal_date"], self.dates[1])
        self.assertEqual(trades[0]["execution_date"], self.dates[2])
        self.assertAlmostEqual(trades[0]["price"], 121.0, places=6)

        # T+2（d3）起吃收益：121 -> 133.1 = +10%。
        self.assertAlmostEqual(daily[self.dates[3]]["nav"], 1.1, places=6)
        # d4 B 暴涨至 300：持仓合法吃掉该收益。
        self.assertAlmostEqual(daily[self.dates[4]]["nav"], 1.1 * (300 / 133.1), places=6)

    def test_determinism_and_input_hash(self):
        first = run_c6_replay(self.assets, self.bench, FAST_PARAMS)
        second = run_c6_replay(self.assets, self.bench, FAST_PARAMS)
        self.assertEqual(first["input_hash"], second["input_hash"])
        self.assertEqual(first["daily"], second["daily"])
        self.assertEqual(first["trades"], second["trades"])


class TestPreListingMask(unittest.TestCase):
    """上市前价格必须被掩蔽，且不得影响信号与成交。"""

    def test_bars_before_list_date_are_masked(self):
        dates = make_dates(6)
        assets = [
            make_asset("AAA", [100] * 6, dates, "A"),
            # C 在 list_date 之前有 3 根「翻倍」合成价格，必须被掩蔽。
            make_asset("CCC", [100, 200, 400, 800, 1600, 3200], dates, "C",
                       list_date=dates[3]),
        ]
        result = run_c6_replay(assets, make_benchmark([100] * 6, dates),
                               {**FAST_PARAMS, "top_n": 2})
        per_asset = {item["code"]: item for item in result["quality"]["per_asset"]}
        self.assertEqual(per_asset["CCC"]["masked_pre_listing_bars"], 3)

        c_trades = [t for t in result["trades"] if t["code"] == "CCC"]
        # 掩蔽后 C 的 lookback 基点最早也要 d3，信号最早 d4 收盘产生，d5 收盘成交。
        self.assertTrue(c_trades, "掩蔽后 C 仍应在数据充足后参与轮动")
        self.assertTrue(all(t["execution_date"] >= dates[5] for t in c_trades))
        # d2 之前绝不能出现 C 的持仓。
        early_positions = [p for p in result["positions"]
                           if p["code"] == "CCC" and p["date"] < dates[5]]
        self.assertEqual(early_positions, [])


class TestMissingPriceHandling(unittest.TestCase):
    """缺价不伪造：持仓缺价按 0 收益并计入质量报告；信号消失则清仓。"""

    def test_missing_bars_flagged_not_fabricated(self):
        dates = make_dates(6)
        assets = [
            # d3 缺日线；d2 收盘 121 买入后，d3/d4 都无法计算真实收益。
            make_asset("AAA", [100, 110, 121, None, 146.41, 161.051], dates, "A"),
        ]
        result = run_c6_replay(assets, make_benchmark([100] * 6, dates), FAST_PARAMS)
        self.assertEqual(result["quality"]["held_missing_price_events"], 2)
        self.assertIn("持仓缺价", "".join(result["quality"]["warnings"]))

        daily = {row["date"]: row for row in result["daily"]}
        # d3、d4 缺价按 0 收益：净值停在 1.0（买入后未吃到任何可验证收益）。
        self.assertAlmostEqual(daily[dates[3]]["nav"], 1.0, places=6)
        self.assertAlmostEqual(daily[dates[4]]["nav"], 1.0, places=6)

        # d3 信号无效 → 空篮子 → d4 收盘清仓。
        sells = [t for t in result["trades"] if t["side"] == "SELL"]
        self.assertEqual(len(sells), 1)
        self.assertEqual(sells[0]["execution_date"], dates[4])
        self.assertAlmostEqual(sells[0]["price"], 146.41, places=6)


class TestRiskLayers(unittest.TestCase):
    def test_timing_flips_to_cash(self):
        dates = make_dates(6)
        params = {**FAST_PARAMS, "use_timing": True, "timing_ma": 2, "lookback": 1}
        assets = [make_asset("AAA", [100, 110, 121, 133.1, 146.41, 161.051], dates, "A")]
        # d4 基准跌破 MA2 -> d4 收盘转现金；d5 继续低于均线，保持空仓。
        bench = make_benchmark([100, 100, 100, 100, 90, 85], dates)
        result = run_c6_replay(assets, bench, params)

        sells = [t for t in result["trades"] if t["side"] == "SELL"]
        self.assertEqual(len(sells), 1)
        self.assertEqual(sells[0]["execution_date"], dates[4])
        self.assertEqual(sells[0]["reason"], "沪深300择时·转现金")

        # 转现金后（risk_free=0）净值冻结；in_market 标志翻转。
        daily = {row["date"]: row for row in result["daily"]}
        self.assertFalse(daily[dates[4]]["in_market"])
        self.assertAlmostEqual(daily[dates[5]]["nav"], daily[dates[4]]["nav"], places=6)
        self.assertAlmostEqual(daily[dates[5]]["cash_weight"], 1.0, places=6)

    def test_trailing_stop_loss(self):
        dates = make_dates(6)
        params = {**FAST_PARAMS, "use_stop": True, "stop_pct": 0.10, "lookback": 2}
        # lookback=2 保证 d4 大跌时信号仍为正，仅止损触发清仓。
        assets = [make_asset("AAA", [100, 100, 121, 120, 100, 100], dates, "A")]
        result = run_c6_replay(assets, make_benchmark([100] * 6, dates), params)

        sells = [t for t in result["trades"] if t["side"] == "SELL"]
        self.assertEqual(len(sells), 1)
        self.assertEqual(sells[0]["reason"], "个股跟踪止损·清仓")
        self.assertEqual(sells[0]["execution_date"], dates[4])
        self.assertAlmostEqual(sells[0]["price"], 100.0, places=6)


class TestMetrics(unittest.TestCase):
    def test_basic_metrics(self):
        metrics = compute_metrics([1.0, 1.1], [1.0, 1.0], axis_dates=["2026-01-05", "2026-01-06"])
        self.assertAlmostEqual(metrics["cum"], 0.1, places=6)
        self.assertAlmostEqual(metrics["mdd"], 0.0, places=6)

    def test_drawdown(self):
        metrics = compute_metrics([1.0, 1.2, 0.9], [1.0, 1.0, 1.0])
        self.assertAlmostEqual(metrics["mdd"], 0.9 / 1.2 - 1, places=6)

    def test_short_sample_degrades_annualized_metrics(self):
        metrics = compute_metrics([1.0, 1.01, 1.02], [1.0, 1.0, 1.0])
        self.assertIsNone(metrics["ann"])
        self.assertIsNone(metrics["sharpe"])
        self.assertIsNone(metrics["beta"])

    def test_month_and_ytd_windows(self):
        dates = ["2026-01-30", "2026-02-02", "2026-02-03"]
        metrics = compute_metrics([1.0, 1.1, 1.21], [1.0, 1.0, 1.0], axis_dates=dates)
        self.assertAlmostEqual(metrics["mth"], 1.21 / 1.1 - 1, places=6)
        self.assertAlmostEqual(metrics["ytd"], 0.21, places=6)


class TestValidation(unittest.TestCase):
    def setUp(self):
        self.dates = make_dates(4)
        self.bench = make_benchmark([100] * 4, self.dates)

    def test_empty_assets_rejected(self):
        with self.assertRaises(ValueError):
            run_c6_replay([], self.bench, FAST_PARAMS)

    def test_duplicate_codes_rejected(self):
        assets = [make_asset("AAA", [100] * 4, self.dates),
                  make_asset("AAA", [100] * 4, self.dates)]
        with self.assertRaises(ValueError):
            run_c6_replay(assets, self.bench, FAST_PARAMS)

    def test_empty_benchmark_rejected(self):
        assets = [make_asset("AAA", [100] * 4, self.dates)]
        with self.assertRaises(ValueError):
            run_c6_replay(assets, {"code": "510300", "bars": []}, FAST_PARAMS)

    def test_invalid_params_rejected(self):
        assets = [make_asset("AAA", [100] * 4, self.dates)]
        with self.assertRaises(ValueError):
            run_c6_replay(assets, self.bench, {**FAST_PARAMS, "top_n": 0})

    def test_locked_universe_shape(self):
        self.assertEqual(len(C6_UNIVERSE), 25)
        self.assertEqual(C6_BENCHMARK["code"], "510300")
        self.assertEqual(DEFAULT_PARAMS["top_n"], 3)
        self.assertEqual(DEFAULT_PARAMS["rebalance_interval"], 5)
        self.assertEqual(DEFAULT_PARAMS["lookback"], 20)
        self.assertTrue(DEFAULT_PARAMS["use_timing"])
        self.assertEqual(DEFAULT_PARAMS["timing_ma"], 120)
        self.assertTrue(DEFAULT_PARAMS["use_stop"])
        self.assertAlmostEqual(DEFAULT_PARAMS["stop_pct"], 0.10)


if __name__ == "__main__":
    unittest.main()
