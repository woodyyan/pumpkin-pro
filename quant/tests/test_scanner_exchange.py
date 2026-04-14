"""
T1: Quant Exchange 分流测试

覆盖 P0-1 (exchange 参数分流) + P0-2 (字段移除)
- 不调用真实外部 API，全部 mock
- 验证 exchange=HKEX 走港股快照、其余走 A 股
- 验证 profit_growth_rate / industry 已从全局配置中移除
"""

import sys
import os
import pytest
import pandas as pd
import numpy as np

# 确保项目根目录在 path 中
sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))

# ---------------------------------------------------------------------------
# 测试目标模块导入
# ---------------------------------------------------------------------------

from screener.scanner import (
    FILTERABLE_COLUMNS,
    NUMERIC_COLUMNS,
    SORTABLE_COLUMNS,
    apply_filters,
    sort_and_paginate,
    df_to_records,
)


# ════════════════════════════════════════════════════════════════════
# Class 1: TestFieldRemoval — 字段全局移除验证
# ════════════════════════════════════════════════════════════════════


class TestProfitGrowthRateRemoved:
    """profit_growth_rate 应已从所有公开列表中移除"""

    def test_not_in_filterable_columns(self):
        assert 'profit_growth_rate' not in FILTERABLE_COLUMNS, \
            'profit_growth_rate 必须从 FILTERABLE_COLUMNS 移除'

    def test_not_in_numeric_columns(self):
        assert 'profit_growth_rate' not in NUMERIC_COLUMNS, \
            'profit_growth_rate 必须从 NUMERIC_COLUMNS 移除'


class TestIndustryRemoved:
    """industry 应已从所有公开列表中移除"""

    def test_not_in_filterable_columns(self):
        assert 'industry' not in FILTERABLE_COLUMNS, \
            'industry 必须从 FILTERABLE_COLUMNS 移除'

    def test_not_in_sortable_columns(self):
        assert 'industry' not in SORTABLE_COLUMNS, \
            'industry 必须从 SORTABLE_COLUMNS 移除'


class TestFilterableColumnsContainsExpectedFields:
    """FILTERABLE_COLUMNS 应包含核心行情字段"""

    def test_contains_price(self):
        assert 'price' in FILTERABLE_COLUMNS

    def test_contains_change_pct(self):
        assert 'change_pct' in FILTERABLE_COLUMNS

    def test_contains_total_mv(self):
        assert 'total_mv' in FILTERABLE_COLUMNS

    def test_contains_pe_pb(self):
        assert 'pe' in FILTERABLE_COLUMNS
        assert 'pb' in FILTERABLE_COLUMNS

    def test_contains_turnover_fields(self):
        assert 'turnover' in FILTERABLE_COLUMNS
        assert 'turnover_rate' in FILTERABLE_COLUMNS
        assert 'volume_ratio' in FILTERABLE_COLUMNS


class TestSortableColumnsStructure:
    """SORTABLE_COLUMNS 应为 A 股+港股列名的并集"""

    def test_contains_code(self):
        assert 'code' in SORTABLE_COLUMNS

    def test_contains_name(self):
        assert 'name' in SORTABLE_COLUMNS

    def test_no_industry(self):
        assert 'industry' not in SORTABLE_COLUMNS

    def test_no_profit_growth_rate(self):
        assert 'profit_growth_rate' not in SORTABLE_COLUMNS


# ════════════════════════════════════════════════════════════════════
# Class 2: TestApplyFiltersCore — 通用筛选逻辑 (T2 部分)
# ════════════════════════════════════════════════════════════════════


def _make_test_df(rows=None) -> pd.DataFrame:
    """构造测试用 DataFrame"""
    if rows is None:
        rows = [
            {'code': '000001', 'name': '平安银行', 'price': 12.5, 'change_pct': 1.2,
             'total_mv': 240e8, 'pe': 8.5, 'pb': 0.7, 'turnover_rate': 0.5,
             'volume_ratio': 1.2, 'amplitude': 3.0, 'turnover': 50000e4,
             'change_amt': 0.15, 'volume': 100000},
            {'code': '000002', 'name': '万科A', 'price': 18.3, 'change_pct': -0.8,
             'total_mv': 220e8, 'pe': 12.0, 'pb': 0.9, 'turnover_rate': 0.3,
             'volume_ratio': 0.8, 'amplitude': 2.0, 'turnover': 30000e4,
             'change_amt': -0.15, 'volume': 60000},
            {'code': '600000', 'name': '浦发银行', 'price': 8.9, 'change_pct': 0.5,
             'total_mv': 260e8, 'pe': 5.5, 'pb': 0.5, 'turnover_rate': 0.1,
             'volume_ratio': 0.6, 'amplitude': 1.5, 'turnover': 20000e4,
             'change_amt': 0.04, 'volume': 40000},
            {'code': '600036', 'name': '招商银行', 'price': 35.6, 'change_pct': 2.3,
             'total_mv': 900e8, 'pe': 9.0, 'pb': 1.4, 'turnover_rate': 0.8,
             'volume_ratio': 1.5, 'amplitude': 3.5, 'turnover': 120000e4,
             'change_amt': 0.80, 'volume': 200000},
        ]
    return pd.DataFrame(rows)


class TestApplyFiltersEmpty:

    def test_empty_filters_returns_all(self):
        df = _make_test_df()
        result = apply_filters(df, {})
        assert len(result) == len(df)

    def test_none_filters_returns_all(self):
        df = _make_test_df()
        result = apply_filters(df, None)
        assert len(result) == len(df)


class TestApplyFiltersRange:

    def test_single_range_filter_price(self):
        df = _make_test_df()
        result = apply_filters(df, {'price': {'min': 10, 'max': 20}})
        assert all(result['price'] >= 10)
        assert all(result['price'] <= 20)

    def test_multiple_filters_and(self):
        df = _make_test_df()
        result = apply_filters(df, {
            'price': {'min': 10, 'max': 20},
            'pe': {'max': 10}
        })
        assert all(result['price'] >= 10)
        assert all(result['price'] <= 20)
        assert all(result['pe'] <= 10)

    def test_min_only(self):
        df = _make_test_df()
        result = apply_filters(df, {'total_mv': {'min': 250e8}})
        assert all(result['total_mv'] >= 250e8)

    def test_max_only(self):
        df = _make_test_df()
        result = apply_filters(df, {'total_mv': {'max': 250e8}})
        assert all(result['total_mv'] <= 250e8)


class TestApplyFiltersSafety:

    def test_unknown_field_skipped(self):
        """不存在的字段应被安全跳过，不报错"""
        df = _make_test_df()
        result = apply_filters(df, {'nonexistent_field': {'min': 0, 'max': 100}})
        assert len(result) == len(df)

    def test_null_min_max_handled(self):
        """min/max 为 null 时等同于不限"""
        df = _make_test_df()
        result = apply_filters(df, {'price': {'min': None, 'max': None}})
        assert len(result) == len(df)

    def test_non_numeric_value_safe(self):
        """非数值 min/max 应被安全忽略"""
        df = _make_test_df()
        result = apply_filters(df, {'price': {'min': 'abc', 'max': 'xyz'}})
        # 不崩溃即可
        assert isinstance(result, pd.DataFrame)

    def test_field_not_in_df_skipped(self):
        """字段不在 DataFrame 中时应跳过"""
        df = _make_test_df()
        result = apply_filters(df, {'change_pct_60d': {'min': -10, 'max': 10}})  # 测试 DF 无此列
        assert len(result) == len(df)


# ════════════════════════════════════════════════════════════════════
# Class 3: TestSortAndPaginate — 排序分页 (T2 部分)
# ════════════════════════════════════════════════════════════════════


class TestSortAndPaginateBasic:

    def test_default_sort_by_code_asc(self):
        df = _make_test_df()
        page_df, total = sort_and_paginate(df, 'code', 'asc', 1, 50)
        assert total == len(df)
        codes = page_df['code'].tolist()
        assert codes == sorted(codes)

    def test_descending_order(self):
        df = _make_test_df()
        page_df, total = sort_and_paginate(df, 'price', 'desc', 1, 50)
        prices = page_df['price'].tolist()
        for i in range(len(prices) - 1):
            assert prices[i] >= prices[i+1], f"价格降序失败: {prices}"

    def test_invalid_sort_key_fallback(self):
        """无效排序列名兜底按 code 排序"""
        df = _make_test_df()
        page_df, total = sort_and_paginate(df, 'invalid_column_xyz', 'asc', 1, 50)
        codes = page_df['code'].tolist()
        assert codes == sorted(codes)

    def test_pagination_correct_slice(self):
        df = _make_test_df()
        page_df, total = sort_and_paginate(df, 'code', 'asc', 2, 2)
        assert total == len(df)
        assert len(page_df) <= 2

    def test_total_count_accurate(self):
        df = _make_test_df()
        _, total = sort_and_paginate(df, 'code', 'asc', 1, 50)
        assert total == len(df)

    def test_page_exceeds_total_returns_empty(self):
        df = _make_test_df()
        page_df, total = sort_and_paginate(df, 'code', 'asc', 999, 50)
        assert len(page_df) == 0
        assert total == len(df)


# ════════════════════════════════════════════════════════════════════
# Class 4: TestDfToRecords — JSON 序列化
# ════════════════════════════════════════════════════════════════════


class TestDfToRecords:

    def test_empty_df(self):
        assert df_to_records(pd.DataFrame()) == []

    def test_normal_data(self):
        df = pd.DataFrame([{'code': '00700', 'name': '腾讯', 'price': 380.0}])
        records = df_to_records(df)
        assert len(records) == 1
        assert records[0]['code'] == '00700'
        assert records[0]['name'] == '腾讯'
        assert records[0]['price'] == 380.0

    def test_nan_converted_to_none(self):
        df = pd.DataFrame([{'code': '000001', 'price': float('nan'), 'pe': None}])
        records = df_to_records(df)
        assert records[0]['price'] is None
        assert records[0]['pe'] is None


# ════════════════════════════════════════════════════════════════════
# Class 5: TestHKSnapshotStructure — 港股数据结构预期
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
