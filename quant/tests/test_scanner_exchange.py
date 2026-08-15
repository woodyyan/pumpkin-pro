"""
scanner 快照结构测试

选股器（/api/screener/scan）已下线，apply_filters / sort_and_paginate /
df_to_records / FILTERABLE_COLUMNS / SORTABLE_COLUMNS 已随之删除。
本文件仅保留快照数据结构相关用例（NUMERIC_COLUMNS / HK_COLUMN_MAP /
HK_NUMERIC_COLUMNS），这些列定义仍被四象限（quadrant.py）的快照链路使用。
"""

import sys
import os

# 确保项目根目录在 path 中
sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))

# ---------------------------------------------------------------------------
# 测试目标模块导入
# ---------------------------------------------------------------------------

from screener.scanner import NUMERIC_COLUMNS


class TestProfitGrowthRateRemoved:
    """profit_growth_rate 应已从公开列配置中移除"""

    def test_not_in_numeric_columns(self):
        assert 'profit_growth_rate' not in NUMERIC_COLUMNS, \
            'profit_growth_rate 必须从 NUMERIC_COLUMNS 移除'


# ════════════════════════════════════════════════════════════════════
# TestHKSnapshotStructure — 港股数据结构预期
# ════════════════════════════════════════════════════════════════════


class TestHKSnapshotStructure:
    """验证港股快照的列结构符合预期"""

    def test_hk_snapshot_has_required_price_fields(self):
        from screener.scanner import HK_COLUMN_MAP
        expected_keys = set(HK_COLUMN_MAP.values())
        assert 'code' in expected_keys
        assert 'name' in expected_keys
        assert 'price' in expected_keys
        assert 'change_pct' in expected_keys
        assert 'total_mv' in expected_keys
        assert 'pe' in expected_keys
        assert 'pb' in expected_keys

    def test_hk_column_map_no_industry(self):
        """港股列名映射不含 industry"""
        from screener.scanner import HK_COLUMN_MAP
        values = set(HK_COLUMN_MAP.values())
        assert 'industry' not in values

    def test_hk_column_map_no_profit_growth_rate(self):
        """港股列名映射不含 profit_growth_rate"""
        from screener.scanner import HK_COLUMN_MAP
        values = set(HK_COLUMN_MAP.values())
        assert 'profit_growth_rate' not in values

    def test_hk_numeric_columns_no_profit_growth_rate(self):
        from screener.scanner import HK_NUMERIC_COLUMNS
        assert 'profit_growth_rate' not in HK_NUMERIC_COLUMNS
        assert 'industry' not in HK_NUMERIC_COLUMNS
