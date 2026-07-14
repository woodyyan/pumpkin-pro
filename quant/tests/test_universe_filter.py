"""
universe_filter 单元测试 — 股票池预过滤模块。

覆盖：
- is_bse_code: 北交所代码判定（含边界用例）
- is_st_name: ST/退市名称判定（含变体）
- is_suspended_row: 停牌代理判定
- filter_a_share_universe: 组合过滤 + 统计 + 熔断
"""

import sys
import os
import pytest
import pandas as pd

# 确保 quant/ 在 sys.path 中
sys.path.insert(0, os.path.join(os.path.dirname(os.path.dirname(os.path.abspath(__file__)))))

from screener.universe_filter import (
    is_bse_code,
    is_st_name,
    is_suspended_row,
    filter_a_share_universe,
    FilterOptions,
    FilterStats,
    MAX_FILTER_RATIO,
)


# ── is_bse_code ────────────────────────────────────────────────

class TestIsBseCode:
    def test_bse_codes(self):
        assert is_bse_code("830799") is True
        assert is_bse_code("870866") is True
        assert is_bse_code("920002") is True
        assert is_bse_code("430047") is True

    def test_non_bse_codes(self):
        assert is_bse_code("600000") is False
        assert is_bse_code("000001") is False
        assert is_bse_code("300750") is False
        assert is_bse_code("688981") is False

    def test_empty_and_invalid(self):
        assert is_bse_code("") is False
        assert is_bse_code(None) is False
        assert is_bse_code("   ") is False

    def test_with_whitespace(self):
        assert is_bse_code("  830799  ") is True
        assert is_bse_code("  600000  ") is False


# ── is_st_name ─────────────────────────────────────────────────

class TestIsStName:
    def test_plain_st(self):
        assert is_st_name("ST天宝") is True
        assert is_st_name("ST生化") is True

    def test_star_st(self):
        assert is_st_name("*ST天宝") is True
        assert is_st_name("*ST生化") is True

    def test_sst_and_s_star_st(self):
        assert is_st_name("SST天宝") is True
        assert is_st_name("S*ST天宝") is True

    def test_delisting(self):
        assert is_st_name("退市美都") is True
        assert is_st_name("退市刚泰") is True

    def test_normal_names(self):
        assert is_st_name("贵州茅台") is False
        assert is_st_name("比亚迪") is False
        assert is_st_name("宁德时代") is False

    def test_empty_and_none(self):
        assert is_st_name("") is False
        assert is_st_name(None) is False

    def test_space_in_name(self):
        # 名称含空格也应正确识别
        assert is_st_name("ST 天宝") is True
        assert is_st_name("* ST生化") is True


# ── is_suspended_row ───────────────────────────────────────────

class TestIsSuspendedRow:
    def test_zero_volume(self):
        assert is_suspended_row(0) is True
        assert is_suspended_row(0.0) is True

    def test_positive_volume(self):
        assert is_suspended_row(1000) is False
        assert is_suspended_row(0.01) is False

    def test_none_volume(self):
        # 保守原则：None 不过滤
        assert is_suspended_row(None) is False

    def test_invalid_types(self):
        assert is_suspended_row("abc") is False
        assert is_suspended_row("") is False


# ── filter_a_share_universe ────────────────────────────────────

class TestFilterAShareUniverse:
    def _make_snapshot(self, rows):
        """构建测试用快照 DataFrame。"""
        return pd.DataFrame(rows)

    def test_basic_filtering(self):
        """正常场景：混合 ST/北交所/停牌/正常股票。"""
        df = self._make_snapshot([
            {"code": "600000", "name": "浦发银行", "price": 10.0, "volume": 1000000},
            {"code": "000001", "name": "平安银行", "price": 12.0, "volume": 2000000},
            {"code": "830799", "name": "正常北交所股", "price": 5.0, "volume": 500000},  # BSE
            {"code": "600001", "name": "ST天宝", "price": 3.0, "volume": 800000},  # ST
            {"code": "000002", "name": "停牌股", "price": 8.0, "volume": 0},  # suspended
        ])
        filtered, stats = filter_a_share_universe(df)
        assert stats.total_before == 5
        assert stats.total_after == 2  # 只剩浦发银行和平安银行
        assert stats.excluded_bse == 1
        assert stats.excluded_st == 1
        assert stats.excluded_suspended == 1

    def test_overlap(self):
        """同时命中多个条件的股票。"""
        df = self._make_snapshot([
            {"code": "830799", "name": "*ST北交所股", "price": 1.0, "volume": 0},  # BSE + ST + suspended
            {"code": "600000", "name": "浦发银行", "price": 10.0, "volume": 1000000},
        ])
        filtered, stats = filter_a_share_universe(df)
        assert stats.total_before == 2
        assert stats.total_after == 1
        assert stats.excluded_bse == 1
        assert stats.excluded_st == 1
        assert stats.excluded_suspended == 1
        assert stats.excluded_overlap == 2  # 3 conditions - 1 actual excluded = 2 overlap

    def test_empty_dataframe(self):
        df = pd.DataFrame(columns=["code", "name", "volume"])
        filtered, stats = filter_a_share_universe(df)
        assert stats.total_before == 0
        assert stats.total_after == 0

    def test_none_dataframe(self):
        filtered, stats = filter_a_share_universe(None)
        assert stats.total_before == 0

    def test_missing_name_column(self):
        """name 列缺失时 ST 过滤降级为不过滤。"""
        df = self._make_snapshot([
            {"code": "600000", "price": 10.0, "volume": 1000000},
            {"code": "830799", "price": 5.0, "volume": 500000},  # BSE 仍然过滤
        ])
        filtered, stats = filter_a_share_universe(df)
        assert stats.excluded_st == 0
        assert stats.excluded_bse == 1
        assert stats.total_after == 1

    def test_missing_volume_column(self):
        """volume 列缺失时停牌过滤降级为不过滤。"""
        df = self._make_snapshot([
            {"code": "600000", "name": "浦发银行", "price": 10.0},
            {"code": "600001", "name": "ST天宝", "price": 3.0},  # ST 仍然过滤
        ])
        filtered, stats = filter_a_share_universe(df)
        assert stats.excluded_suspended == 0
        assert stats.excluded_st == 1
        assert stats.total_after == 1

    def test_nan_name(self):
        """name 为 NaN 时不过滤该行的 ST。"""
        df = self._make_snapshot([
            {"code": "600000", "name": float("nan"), "price": 10.0, "volume": 1000000},
            {"code": "600001", "name": "ST天宝", "price": 3.0, "volume": 800000},
        ])
        filtered, stats = filter_a_share_universe(df)
        assert stats.excluded_st == 1
        assert stats.total_after == 1  # NaN name 的不过滤，ST 的过滤

    def test_nan_volume(self):
        """volume 为 NaN 时不过滤该行的停牌。"""
        df = self._make_snapshot([
            {"code": "600000", "name": "浦发银行", "price": 10.0, "volume": float("nan")},
            {"code": "600001", "name": "停牌股", "price": 3.0, "volume": 0},
        ])
        filtered, stats = filter_a_share_universe(df)
        assert stats.excluded_suspended == 1  # volume=0 的被过滤
        assert stats.total_after == 1  # NaN volume 的不过滤

    def test_filter_options_disable_bse(self):
        """关闭北交所过滤。"""
        df = self._make_snapshot([
            {"code": "830799", "name": "北交所股", "price": 5.0, "volume": 500000},
            {"code": "600000", "name": "浦发银行", "price": 10.0, "volume": 1000000},
        ])
        filtered, stats = filter_a_share_universe(df, FilterOptions(exclude_bse=False))
        assert stats.excluded_bse == 0
        assert stats.total_after == 2

    def test_filter_options_disable_st(self):
        """关闭 ST 过滤。"""
        df = self._make_snapshot([
            {"code": "600001", "name": "ST天宝", "price": 3.0, "volume": 800000},
            {"code": "600000", "name": "浦发银行", "price": 10.0, "volume": 1000000},
        ])
        filtered, stats = filter_a_share_universe(df, FilterOptions(exclude_st=False))
        assert stats.excluded_st == 0
        assert stats.total_after == 2

    def test_filter_options_disable_suspended(self):
        """关闭停牌过滤。"""
        df = self._make_snapshot([
            {"code": "600001", "name": "停牌股", "price": 3.0, "volume": 0},
            {"code": "600000", "name": "浦发银行", "price": 10.0, "volume": 1000000},
        ])
        filtered, stats = filter_a_share_universe(df, FilterOptions(exclude_suspended=False))
        assert stats.excluded_suspended == 0
        assert stats.total_after == 2

    def test_ratio_alert_not_triggered(self):
        """正常过滤比例不触发告警。"""
        rows = [{"code": f"60000{i}", "name": f"股票{i}", "price": 10.0, "volume": 1000000} for i in range(10)]
        rows.append({"code": "830799", "name": "北交所", "price": 5.0, "volume": 500000})
        df = self._make_snapshot(rows)
        _, stats = filter_a_share_universe(df)
        assert stats.ratio_alert is False

    def test_ratio_alert_triggered(self):
        """过滤比例超过阈值触发告警。"""
        rows = [{"code": f"8307{i}", "name": f"北交所{i}", "price": 5.0, "volume": 500000} for i in range(20)]
        rows.append({"code": "600000", "name": "浦发银行", "price": 10.0, "volume": 1000000})
        df = self._make_snapshot(rows)
        _, stats = filter_a_share_universe(df)
        assert stats.ratio_alert is True
        assert stats.filter_ratio > MAX_FILTER_RATIO

    def test_stats_to_dict(self):
        """FilterStats.to_dict() 返回完整字段。"""
        stats = FilterStats(
            total_before=100,
            total_after=90,
            excluded_bse=5,
            excluded_st=3,
            excluded_suspended=2,
            excluded_overlap=0,
            filter_ratio=0.1,
            ratio_alert=False,
        )
        d = stats.to_dict()
        assert d["total_before"] == 100
        assert d["total_after"] == 90
        assert d["excluded_bse"] == 5
        assert d["excluded_st"] == 3
        assert d["excluded_suspended"] == 2
        assert d["filter_ratio"] == 0.1
        assert d["ratio_alert"] is False
