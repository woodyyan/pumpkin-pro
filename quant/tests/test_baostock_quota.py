"""
GlobalBaostockQuotaGuard 单元测试。

覆盖：
- 基本 try_acquire + snapshot
- 配额耗尽后拒绝
- 黑名单触发与检查
- caller 归因统计
- 跨实例单例行为
"""

import sys
import os
import tempfile
import pytest

sys.path.insert(0, os.path.join(os.path.dirname(os.path.dirname(os.path.abspath(__file__)))))

from data_sources.quota.baostock_quota import GlobalBaostockQuotaGuard


@pytest.fixture
def quota_guard(tmp_path):
    """每次测试用独立的临时 DB 文件 + 低配额便于测试。"""
    db_path = str(tmp_path / "test_quota.db")
    # 重置单例
    GlobalBaostockQuotaGuard._instance = None
    guard = GlobalBaostockQuotaGuard(db_path=db_path, daily_quota=100)
    yield guard
    GlobalBaostockQuotaGuard._instance = None


class TestTryAcquire:
    def test_basic_acquire(self, quota_guard):
        assert quota_guard.try_acquire(cost=1, caller="test") is True
        snap = quota_guard.snapshot()
        assert snap["used_count"] == 1
        assert snap["remaining"] == 99

    def test_multiple_acquires(self, quota_guard):
        for i in range(10):
            assert quota_guard.try_acquire(cost=1, caller="test") is True
        snap = quota_guard.snapshot()
        assert snap["used_count"] == 10
        assert snap["remaining"] == 90

    def test_exhaust_quota(self, quota_guard):
        """配额耗尽后拒绝。"""
        assert quota_guard.try_acquire(cost=100, caller="test") is True
        assert quota_guard.try_acquire(cost=1, caller="test") is False
        snap = quota_guard.snapshot()
        assert snap["used_count"] == 100
        assert snap["remaining"] == 0

    def test_partial_reject(self, quota_guard):
        """余量不足时拒绝。"""
        assert quota_guard.try_acquire(cost=95, caller="test") is True
        assert quota_guard.try_acquire(cost=10, caller="test") is False  # 只剩 5，要 10
        snap = quota_guard.snapshot()
        assert snap["used_count"] == 95


class TestBlacklist:
    def test_blacklist_triggered_at_threshold(self, quota_guard):
        """达到熔断阈值（90%）时自动黑名单。"""
        # threshold = 100 * 0.90 = 90
        assert quota_guard.try_acquire(cost=90, caller="test") is True
        snap = quota_guard.snapshot()
        assert snap["blacklisted"] is True
        assert snap["used_count"] == 90

    def test_blacklisted_rejects_subsequent(self, quota_guard):
        """黑名单后拒绝所有后续请求。"""
        quota_guard.try_acquire(cost=90, caller="test")  # 触发黑名单
        assert quota_guard.try_acquire(cost=1, caller="test") is False

    def test_manual_blacklist(self, quota_guard):
        """手动标记黑名单。"""
        quota_guard.mark_blacklisted("manual test")
        assert quota_guard.is_blacklisted() is True
        assert quota_guard.try_acquire(cost=1, caller="test") is False

    def test_is_blacklisted_false_initially(self, quota_guard):
        assert quota_guard.is_blacklisted() is False


class TestCallerAttribution:
    def test_by_caller_snapshot(self, quota_guard):
        """snapshot 返回 by_caller 统计。"""
        quota_guard.try_acquire(cost=5, caller="quadrant")
        quota_guard.try_acquire(cost=3, caller="factor_lab")
        quota_guard.try_acquire(cost=2, caller="quadrant")
        snap = quota_guard.snapshot()
        assert snap["by_caller"]["quadrant"] == 7
        assert snap["by_caller"]["factor_lab"] == 3

    def test_unknown_caller(self, quota_guard):
        """默认 caller 为 unknown。"""
        quota_guard.try_acquire(cost=1)
        snap = quota_guard.snapshot()
        assert snap["by_caller"].get("unknown") == 1


class TestSnapshot:
    def test_empty_snapshot(self, quota_guard):
        """无消耗时 snapshot 返回零值。"""
        snap = quota_guard.snapshot()
        assert snap["used_count"] == 0
        assert snap["daily_quota"] == 100
        assert snap["remaining"] == 100
        assert snap["blacklisted"] is False
        assert snap["by_caller"] == {}
        assert snap["usage_ratio"] == 0.0

    def test_usage_ratio(self, quota_guard):
        quota_guard.try_acquire(cost=50, caller="test")
        snap = quota_guard.snapshot()
        assert snap["usage_ratio"] == 0.5
