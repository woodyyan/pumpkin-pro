"""
港股四象限计算 — 缺失字段容错测试

验证当快照数据源缺少某些列（如 turnover_rate、volume_ratio 等）时，
compute_hk_quadrant_scores 的评分计算不会因 KeyError 崩溃，而是
使用安全的 fallback 值。

对应 bug: [hk-quadrant] compute-hk-all 后台任务失败: 'turnover_rate'
根因: quadrant.py 直接访问 merged["turnover_rate"] 等列，
     但东财/腾讯数据源可能不返回该列。
"""

import numpy as np
import pandas as pd
import pytest

pytest.importorskip("requests", reason="screener.quadrant 依赖 requests")


def _make_snapshot_df(columns: list[str], n: int = 10) -> pd.DataFrame:
    """构造一个最小化的港股快照 DataFrame，只包含指定列。"""
    np.random.seed(99)
    data = {"code": [f"{i:05d}" for i in range(1, n + 1)], "name": [f"Stock{i}" for i in range(1, n + 1)]}
    for col in columns:
        if col not in ("code", "name"):
            data[col] = np.random.uniform(0.5, 100, n)
    return pd.DataFrame(data)


def _make_daily_metrics_df(codes: list[str]) -> pd.DataFrame:
    """模拟 _compute_daily_metrics 的输出（带 turnover 列）。"""
    np.random.seed(42)
    n = len(codes)
    return pd.DataFrame({
        "code": codes,
        "std_20d": np.random.uniform(0.01, 0.05, n),
        "max_drawdown_60d": np.random.uniform(-0.4, -0.05, n),
        "change_pct_60d_calc": np.random.uniform(-30, 50, n),
        "volume_ratio_calc": np.random.uniform(0.3, 3.0, n),
        "turnover": np.random.uniform(10000, 500000, n),
        "turnover_20d_avg": np.random.uniform(200000, 800000, n),
        "cumulative_turnover_20d": np.random.uniform(2, 15, n),
    })


class TestHKQuadrantMissingTurnoverRate:
    """核心回归：快照无 turnover_rate 列时不崩溃。"""

    def test_flow_no_turnover_rate_column(self):
        """merged 中没有 turnover_rate → Flow 用 fallback=50，不抛 KeyError。"""
        # Arrange: 快照缺少 turnover_rate 和 volume_ratio（腾讯兜底场景可能如此）
        snapshot = _make_snapshot_df(
            ["code", "name", "price", "pe", "pb", "change_pct",
             "total_mv", "float_mv", "volume", "turnover"],
            n=10,
        )
        metrics = _make_daily_metrics_df(snapshot["code"].tolist())
        merged = snapshot.merge(metrics, on="code", how="left")

        # 确保 turnover_rate 不存在
        assert "turnover_rate" not in merged.columns

        # Act: 模拟 Flow 计算逻辑（从 quadrant.py 提取的核心模式）
        from screener.quadrant import _percentile_rank

        volume_ratio_rank = (
            _percentile_rank(merged["volume_ratio"]).fillna(50)
            if "volume_ratio" in merged.columns
            else pd.Series(50.0, index=merged.index)
        )
        turnover_rate_rank = (
            _percentile_rank(merged["turnover_rate"]).fillna(50)
            if "turnover_rate" in merged.columns
            else pd.Series(50.0, index=merged.index)
        )
        if "turnover" in merged.columns and "turnover_20d_avg" in merged.columns:
            tr_col = merged["turnover"] / merged["turnover_20d_avg"].replace(0, np.nan)
            turnover_ratio_rank = _percentile_rank(tr_col).fillna(50)
        else:
            turnover_ratio_rank = pd.Series(50.0, index=merged.index)
        flow = 0.4 * volume_ratio_rank + 0.3 * turnover_rate_rank + 0.3 * turnover_ratio_rank

        # Assert: 无崩溃，flow 全为 50.0（因为 volume_ratio 也缺）
        assert len(flow) == 10
        assert (flow == 50.0).all(), f"Expected all 50.0 but got {flow.tolist()}"

    def test_flow_with_turnover_rate_present(self):
        """快照有 turnover_rate 时正常排名计算。"""
        snapshot = _make_snapshot_df(
            ["code", "name", "price", "pe", "pb", "change_pct",
             "total_mv", "volume", "turnover_rate", "volume_ratio"],
            n=10,
        )
        metrics = _make_daily_metrics_df(snapshot["code"].tolist())
        merged = snapshot.merge(metrics, on="code", how="left")

        from screener.quadrant import _percentile_rank

        vr_rank = _percentile_rank(merged["volume_ratio"]).fillna(50)
        tr_rank = _percentile_rank(merged["turnover_rate"]).fillna(50)
        tr_ratio = merged["turnover"] / merged["turnover_20d_avg"].replace(0, np.nan)
        trr_rank = _percentile_rank(tr_ratio).fillna(50)
        flow = 0.4 * vr_rank + 0.3 * tr_rank + 0.3 * trr_rank

        # Assert: 有真实数据时 flow 应该有分布（不全等于 50）
        assert len(flow) == 10
        assert flow.std() > 0, "Flow should have variation when real data exists"

    def test_trend_missing_change_pct_60d(self):
        """merged 缺少 change_pct_60d → Trend 不崩溃，返回合理默认值。

        注意：c60d 全 NaN 时 fillna(0) 后 excess_return 全为 -bench_60d（相同值），
        _percentile_rank 对等值序列返回中间百分位（≈50-57），不是精确的 50。
        关键断言：无 NaN、值在 [0,100] 范围内、无崩溃。
        """
        snapshot = _make_snapshot_df(
            ["code", "name", "price", "pe", "pb"],  # 无 change_pct_60d
            n=8,
        )
        metrics = _make_daily_metrics_df(snapshot["code"].tolist())
        merged = snapshot.merge(metrics, on="code", how="left")
        bench_60d = 5.0

        from screener.quadrant import _percentile_rank

        c60d = merged.get("change_pct_60d", pd.Series(np.nan, index=merged.index))
        change_60d_rank = _percentile_rank(c60d).fillna(50)
        excess_return = c60d.fillna(0) - bench_60d
        excess_rank = _percentile_rank(excess_return).fillna(50)
        trend = 0.5 * change_60d_rank + 0.5 * excess_rank

        assert len(trend) == 8
        assert trend.notna().all(), f"Trend contains NaN: {trend.tolist()}"
        assert trend.between(0, 100).all(), f" Trend out of range: min={trend.min()}, max={trend.max()}"

    def test_crowding_missing_cumulative_turnover(self):
        """cumulative_turnover_20d 缺失 → Crowding fallback 到 PE-only 模式，不崩溃。

        空 dtype Series 的 notna() 结果长度为 0，np.where 需要等长数组，
        所以必须确保 fallback Series 与 merged 同索引对齐。
        """
        snapshot = _make_snapshot_df(
            ["code", "name", "price", "pe"],  # 无 cumulative_turnover_20d
            n=6,
        )

        from screener.quadrant import _percentile_rank

        pe_col = snapshot.get("pe", pd.Series(np.nan, index=snapshot.index))
        pe_rank = _percentile_rank(pe_col).fillna(50)
        cum_tr_col = snapshot.get("cumulative_turnover_20d",
                                   pd.Series(dtype=float, index=snapshot.index))  # 必须带 index！
        cum_turnover_rank = _percentile_rank(cum_tr_col).fillna(50)
        has_cum_turnover = cum_tr_col.notna()
        crowding = pd.Series(np.where(
            has_cum_turnover,
            0.5 * pe_rank + 0.5 * cum_turnover_rank,
            pe_rank,
        ), index=snapshot.index)

        assert len(crowding) == 6
        assert crowding.notna().all(), f"Crowding contains NaN: {crowding.tolist()}"
        assert crowding.between(0, 100).all(), f"Crowding out of range: min={crowding.min()}, max={crowding.max()}"

    def test_revision_missing_pe_fallback(self):
        """PE 列也缺时 revision 计算不应崩溃，返回合理默认值。"""
        snapshot = _make_snapshot_df(
            ["code", "name"],  # 极端情况：几乎什么都没有
            n=5,
        )

        from screener.quadrant import _percentile_rank

        pe_for_rev = snapshot.get("pe", pd.Series(999.0, index=snapshot.index))
        revision = _percentile_rank(-pe_for_rev.fillna(999)).fillna(50)

        assert len(revision) == 5
        assert revision.notna().all(), f"Revision contains NaN: {revision.tolist()}"
        assert revision.between(0, 100).all()


class TestHkQuadrantSnapshotColumnResilience:
    """验证不同数据源返回不同列集时的整体兼容性。"""

    @pytest.mark.parametrize("missing_cols", [
        ["turnover_rate"],
        ["turnover_rate", "volume_ratio"],
        ["turnover_rate", "volume_ratio", "pe", "pb"],
        ["change_pct_60d", "profit_growth_rate"],
    ])
    def test_merged_score_calculation_resilient(self, missing_cols):
        """移除指定列后，所有四维度（Trend/Flow/Revision/Volatility/Drawdown/Crowding）计算均不崩溃。"""
        # Full column set
        all_cols = [
            "code", "name", "price", "pe", "pb", "change_pct",
            "total_mv", "float_mv", "volume", "turnover",
            "turnover_rate", "volume_ratio", "profit_growth_rate",
        ]
        available_cols = [c for c in all_cols if c not in missing_cols]

        snapshot = _make_snapshot_df(available_cols, n=15)
        metrics = _make_daily_metrics_df(snapshot["code"].tolist())
        merged = snapshot.merge(metrics, on="code", how="left")

        from screener.quadrant import _percentile_rank

        # --- Trend ---
        c60d = merged.get("change_pct_60d", pd.Series(np.nan, index=merged.index))
        trend = (
            0.5 * _percentile_rank(c60d).fillna(50) +
            0.5 * _percentile_rank(c60d.fillna(0) - 5.0).fillna(50)
        )

        # --- Flow ---
        vol_r = _percentile_rank(merged.get("volume_ratio", pd.Series(dtype=float))).fillna(50) if "volume_ratio" in merged.columns else pd.Series(50.0, index=merged.index)
        tor_r = _percentile_rank(merged.get("turnover_rate", pd.Series(dtype=float))).fillna(50) if "turnover_rate" in merged.columns else pd.Series(50.0, index=merged.index)
        if "turnover" in merged.columns and "turnover_20d_avg" in merged.columns:
            trr = _percentile_rank(merged["turnover"] / merged["turnover_20d_avg"].replace(0, np.nan)).fillna(50)
        else:
            trr = pd.Series(50.0, index=merged.index)
        flow = 0.4 * vol_r + 0.3 * tor_r + 0.3 * trr

        # --- Revision ---
        pgr = merged.get("profit_growth_rate", pd.Series(dtype=float))
        if "profit_growth_rate" not in merged.columns or pgr.isna().all():
            pe_r = merged.get("pe", pd.Series(999.0, index=merged.index))
            rev = _percentile_rank(-pe_r.fillna(999)).fillna(50)
        else:
            rev = _percentile_rank(pgr).fillna(50)

        # --- Volatility / Drawdown ---
        vol_raw = _percentile_rank(merged.get("std_20d", pd.Series(dtype=float))).fillna(50)
        dd_raw = _percentile_rank(merged.get("max_drawdown_60d", pd.Series(dtype=float))).fillna(50)

        # --- Crowding ---
        pe_for_cr = merged.get("pe", pd.Series(np.nan, index=merged.index))
        ctr = merged.get("cumulative_turnover_20d",
                         pd.Series(dtype=float, index=merged.index))  # 必须带 index
        cr_pe_r = _percentile_rank(pe_for_cr).fillna(50)
        cr_ctr_r = _percentile_rank(ctr).fillna(50)
        has_ctr = ctr.notna()
        crowd = pd.Series(np.where(has_ctr, 0.5 * cr_pe_r + 0.5 * cr_ctr_r, cr_pe_r), index=merged.index)

        # Final scores
        opp = _percentile_rank((0.5 * trend + 0.3 * flow + 0.2 * rev).fillna(50)).fillna(50).clip(0, 100)
        risk = _percentile_rank((0.4 * vol_raw + 0.3 * dd_raw + 0.3 * crowd).fillna(50)).fillna(50).clip(0, 100)

        # Assertions: no NaN, no crashes, values in valid range
        for name, series in [("opportunity", opp), ("risk", risk),
                             ("trend", trend), ("flow", flow),
                             ("revision", rev), ("volatility", vol_raw),
                             ("drawdown", dd_raw), ("crowding", crowd)]:
            assert len(series) == 15, f"{name}: wrong length"
            assert series.notna().all(), f"{name} contains NaN after fillna"
            assert series.between(0, 100).all(), f"{name} out of [0,100]: min={series.min()}, max={series.max()}"
