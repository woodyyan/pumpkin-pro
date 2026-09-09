# -*- coding: utf-8 -*-
"""C6 数据加载器测试：日期归一化、去重、失败归因。"""
from __future__ import annotations

import sys
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))

from data_sources.models import Capability, DailyBar, DataSourceResponse, Market  # noqa: E402
from strategy_portfolios.c6_loader import load_c6_inputs  # noqa: E402


class FakeManager:
    """按 symbol 返回预置行情；未配置的 symbol 返回失败。"""

    def __init__(self, bars_by_symbol):
        self.bars_by_symbol = bars_by_symbol
        self.calls = []

    def fetch_daily_bars(self, *, symbol, market, start_date="", end_date="", **kwargs):
        self.calls.append(symbol)
        rows = self.bars_by_symbol.get(symbol)
        if rows is None:
            return DataSourceResponse(ok=False, capability=Capability.DAILY_BARS,
                                      market=str(market), symbol=symbol,
                                      errors=["mock: 无此标的"], data=[], trace=[])
        bars = [DailyBar(symbol=symbol, market=str(market), trade_date=d, open=c,
                         close=c, high=c, low=c, provider="mock")
                for d, c in rows]
        return DataSourceResponse(ok=True, capability=Capability.DAILY_BARS,
                                  market=str(market), symbol=symbol,
                                  data=bars, trace=[], errors=[])


class TestC6Loader(unittest.TestCase):
    def test_normalizes_dates_and_dedupes(self):
        manager = FakeManager({
            "512660": [("20260105", 100.0), ("2026-01-06", 101.0), ("2026-01-06", 102.0)],
            "510300": [("2026-01-05", 4000.0), ("2026-01-06", 4010.0)],
        })
        universe = [{"code": "512660", "name": "军工ETF国泰", "list_date": ""}]
        benchmark = {"code": "510300", "name": "沪深300ETF", "list_date": ""}
        assets, bench, trace = load_c6_inputs(manager, "2026-01-01", "2026-01-31",
                                              universe=universe, benchmark=benchmark)
        self.assertEqual([b["date"] for b in assets[0]["bars"]], ["2026-01-05", "2026-01-06"])
        # 重复日期保留最后一条。
        self.assertEqual(assets[0]["bars"][-1]["close"], 102.0)
        self.assertEqual(len(bench["bars"]), 2)
        self.assertEqual(trace["errors"], [])
        self.assertEqual(len(trace["symbols"]), 2)

    def test_failed_symbol_recorded_not_silently_dropped(self):
        manager = FakeManager({"510300": [("2026-01-05", 4000.0)]})
        universe = [{"code": "512660", "name": "军工ETF国泰", "list_date": ""}]
        assets, _, trace = load_c6_inputs(manager, "", "",
                                          universe=universe,
                                          benchmark={"code": "510300", "name": "B"})
        self.assertEqual(assets[0]["bars"], [])
        self.assertEqual(len(trace["errors"]), 1)
        self.assertIn("512660", trace["errors"][0])


if __name__ == "__main__":
    unittest.main()
