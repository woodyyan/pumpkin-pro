"""
四象限 — 日线拉取成功率过低时的优雅降级测试

对应根因：生产环境腾讯主源被限流 / fallback 不可用时，全量刷新成功率坍塌，
旧逻辑直接 RuntimeError 中止且缓存从未写入，导致每次都重新全量刷新（死循环）。

加固后行为：
1. 即便全量刷新成功率 < 阈值，也把已拉取的部分写入缓存并标记为已全量刷新，
   使下次进入增量模式（轻量），逐步补齐缺口。
2. 若缓存覆盖率已 >= 阈值，降级继续计算（缺失股票以中性分参与），不再硬失败。
3. 若缓存也不足，报错文案带「主数据源异常」主因，便于定位。
4. 增量模式不再强制全量刷新（避免数据源不可用时的死循环）。
5. needs_full_refresh 尊重「近期已尝试全量刷新」，避免空缓存在限流时死循环。
"""

import os
import tempfile

import numpy as np
import pandas as pd
import pytest

pytest.importorskip("requests", reason="screener.quadrant 依赖 requests")


def _make_snapshot(codes, columns):
    np.random.seed(7)
    n = len(codes)
    data = {"code": codes, "name": [f"S{c}" for c in codes]}
    for col in columns:
        if col not in ("code", "name"):
            data[col] = np.random.uniform(1, 100, n)
    return pd.DataFrame(data)


def _stub_manager(fail_daily: bool):
    """fail_daily=True 模拟主源被限流（日线全部失败），指数仍成功。"""
    from data_sources.models import DataSourceResponse, DailyBar

    class StubManager:
        def fetch_daily_bars(self, **kwargs):
            if fail_daily:
                return DataSourceResponse(
                    ok=False,
                    capability="daily_bars",
                    market=kwargs.get("market", "ASHARE"),
                    symbol=kwargs["symbol"],
                    errors=["tencent: HTTP 429 被限流", "eastmoney: 连接超时", "akshare: 连接超时"],
                )
            return DataSourceResponse(
                ok=True,
                capability="daily_bars",
                market=kwargs.get("market", "ASHARE"),
                symbol=kwargs["symbol"],
                data=[DailyBar(symbol=kwargs["symbol"], market=kwargs.get("market", "ASHARE"),
                               trade_date="2026-07-10", open=10, close=11, high=12, low=9, volume=100)],
            )

        def fetch_index_bars(self, **kwargs):
            return DataSourceResponse(
                ok=True,
                capability="index_bars",
                market=kwargs.get("market", "ASHARE"),
                symbol=kwargs["symbol"],
                data=[
                    DailyBar(symbol=kwargs["symbol"], market=kwargs.get("market", "ASHARE"),
                             trade_date="2026-05-01", open=100, close=100, high=101, low=99, volume=1),
                    DailyBar(symbol=kwargs["symbol"], market=kwargs.get("market", "ASHARE"),
                             trade_date="2026-07-10", open=110, close=120, high=121, low=109, volume=1),
                ],
            )

    return StubManager()


def _seeded_cache_factory(base_cls, seed_codes, captured=None):
    """返回一个缓存子类：在自身实例（即 compute 使用的同一实例）内预置部分股票日线，
    避免使用独立连接预置导致的 WAL 跨连接不可见问题。

    captured: 可选列表，会把 compute 内部创建的缓存实例记录进去，
    便于在同一连接上验证 mark_full_refresh 等副作用（规避 WAL 跨连接可见性差异）。
    """
    class _Seeded(base_cls):
        def __init__(self, db_path=None):
            # 不向 super 传递 db_path：
            # - A 股基类 DailyBarCache.__init__(db_path=None) 用默认全局（已被 monkeypatch）
            # - 港股 HkDailyBarCache.__init__() 不接受 db_path，会内部用 HK_CACHE_DB_PATH（已被 monkeypatch）
            super().__init__()
            for code in seed_codes:
                bars = [
                    {"date": f"2026-07-0{d}", "open": 10.0 + d, "close": 11.0 + d,
                     "high": 12.0 + d, "low": 9.0 + d, "volume": 100.0 + d}
                    for d in range(1, 9)
                ]
                self.set_stock_bars(code, bars)
            self.save()
            if captured is not None:
                captured.append(self)
    return _Seeded


class TestQuadrantDegradation:
    def test_a_share_full_refresh_degrades_when_cache_covers(self, monkeypatch):
        """全量刷新日线全部失败，但缓存已覆盖 80% → 降级继续，不抛异常。"""
        import screener.quadrant as quadrant

        tmp = tempfile.mkdtemp()
        db_path = os.path.join(tmp, "quadrant_cache.db")
        monkeypatch.setattr(quadrant, "CACHE_DB_PATH", db_path)

        codes = [f"{i:06d}" for i in range(1, 11)]  # 10 只
        Seeded = _seeded_cache_factory(quadrant.DailyBarCache, codes[:8])  # 预缓存 8 只
        monkeypatch.setattr(quadrant, "DailyBarCache", Seeded)

        snapshot = _make_snapshot(
            codes,
            ["price", "pe", "change_pct", "turnover_rate", "volume_ratio",
             "total_mv", "float_mv", "volume", "turnover", "profit_growth_rate"],
        )
        monkeypatch.setattr(quadrant, "get_a_share_snapshot", lambda: snapshot)
        monkeypatch.setattr(quadrant, "_DATA_SOURCE_MANAGER", _stub_manager(fail_daily=True))

        results = quadrant.compute_all_quadrant_scores(force_full=True)
        assert len(results) == 10
        assert all("opportunity" in r and "risk" in r for r in results)
        # 预缓存的 8 只应有真实日线指标，缺失的 2 只以中性分参与（不崩溃）
        cached_scores = [r["opportunity"] for r in results if r["code"] in set(codes[:8])]
        assert all(0 <= s <= 100 for s in cached_scores)

    def test_a_share_full_refresh_raises_with_dominant_reason_when_cache_empty(self, monkeypatch):
        """全量刷新日线全部失败且缓存为空 → 带主因的 RuntimeError，
        但仍标记「已全量刷新」→ 下次进入增量（打破死循环）。"""
        import screener.quadrant as quadrant

        tmp = tempfile.mkdtemp()
        db_path = os.path.join(tmp, "quadrant_cache.db")
        monkeypatch.setattr(quadrant, "CACHE_DB_PATH", db_path)
        # 空缓存子类（seed_codes=[]），并捕获 compute 内部创建的实例
        captured = []
        Seeded = _seeded_cache_factory(quadrant.DailyBarCache, [], captured)
        monkeypatch.setattr(quadrant, "DailyBarCache", Seeded)

        codes = [f"{i:06d}" for i in range(1, 11)]
        snapshot = _make_snapshot(
            codes,
            ["price", "pe", "change_pct", "turnover_rate", "volume_ratio"],
        )
        monkeypatch.setattr(quadrant, "get_a_share_snapshot", lambda: snapshot)
        monkeypatch.setattr(quadrant, "_DATA_SOURCE_MANAGER", _stub_manager(fail_daily=True))

        with pytest.raises(RuntimeError) as exc:
            quadrant.compute_all_quadrant_scores(force_full=True)
        assert "主数据源异常" in str(exc.value)

        # 关键：即便失败，已标记「已全量刷新」，使下次运行进入增量模式而非再次全量。
        # 用 compute 内部同一连接实例验证（规避 WAL 跨连接可见性差异）。
        assert captured, "compute 未创建缓存实例"
        assert captured[-1].needs_full_refresh() is False

    def test_hk_full_refresh_degrades_when_cache_covers(self, monkeypatch):
        """港股全量刷新日线全部失败，但缓存已覆盖 → 降级继续。"""
        import screener.quadrant as quadrant
        import screener.scanner as scanner

        tmp = tempfile.mkdtemp()
        db_path = os.path.join(tmp, "hk_quadrant_cache.db")
        monkeypatch.setattr(quadrant, "HK_CACHE_DB_PATH", db_path)

        codes = [f"{i:05d}" for i in range(1, 11)]  # 10 只港股
        Seeded = _seeded_cache_factory(quadrant.HkDailyBarCache, codes[:9])  # 预缓存 9 只（>=80%）
        monkeypatch.setattr(quadrant, "HkDailyBarCache", Seeded)

        snapshot = _make_snapshot(
            codes,
            ["price", "pe", "change_pct", "turnover_rate", "volume_ratio",
             "total_mv", "float_mv", "volume", "turnover", "profit_growth_rate"],
        )
        monkeypatch.setattr(scanner, "get_hk_snapshot", lambda: snapshot)
        monkeypatch.setattr(quadrant, "_DATA_SOURCE_MANAGER", _stub_manager(fail_daily=True))

        results = quadrant.compute_hk_quadrant_scores(force_full=True)
        assert len(results) == 10
        assert all("opportunity" in r and "risk" in r for r in results)

    def test_needs_full_refresh_respects_recent_full_attempt(self, monkeypatch):
        """空缓存但近期已标记全量刷新 → 返回 False（走增量），避免限流时死循环。"""
        import screener.quadrant as quadrant

        tmp = tempfile.mkdtemp()
        db_path = os.path.join(tmp, "quadrant_cache.db")
        monkeypatch.setattr(quadrant, "CACHE_DB_PATH", db_path)

        cache = quadrant.DailyBarCache(db_path=db_path)
        # 模拟「刚尝试过一次全量刷新但什么都没拉到」
        cache.mark_full_refresh()
        assert cache.needs_full_refresh() is False  # 空缓存但近期已全量 → 增量
        assert cache.needs_full_refresh(force_full=True) is True  # force 仍强制全量
