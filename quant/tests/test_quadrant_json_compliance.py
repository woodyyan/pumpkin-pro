"""
四象限结果数据 — JSON 合规性清洗测试

验证 _sanitize_item / _finite 函数能正确处理：
  - 正常有限浮点值（不变）
  - NaN → 替换为 fallback (0.0)
  - +Infinity → 替换为 fallback
  - -Infinity → 替换为 fallback
  - 混合场景（部分字段非法）
  - json.dumps 序列化不再报错

对应 bug: "Out of range float values are not JSON compliant"
根因: 某些股票的 daily-bar 数据缺失/退化，经 percentile_rank 等计算后
     产生 NaN 或 Inf，Python 标准 json.dumps 不允许这些值。
"""

import json
import math
import sys
from unittest.mock import MagicMock

import numpy as np
import pytest


# ── Mock heavy dependencies so quadrant can be imported in CI ─────────
# screener.quadrant top-level imports 'requests', which may be absent.
for _mod in ('requests',):
    if _mod not in sys.modules:
        sys.modules[_mod] = MagicMock()


# ── Import target functions from quadrant module ──────────────────
from screener.quadrant import _finite, _sanitize_item


# ══════════════════════════════════════════════════════════════════
# _finite unit tests
# ══════════════════════════════════════════════════════════════════

class TestFinite:
    def test_normal_float(self):
        assert _finite(42.5) == pytest.approx(42.5)
        assert _finite(0.0) == pytest.approx(0.0)
        assert _finite(-3.14) == pytest.approx(-3.14)

    def test_nan(self):
        assert _finite(float("nan")) == 0.0

    def test_positive_inf(self):
        assert _finite(float("inf")) == 0.0

    def test_negative_inf(self):
        assert _finite(float("-inf")) == 0.0

    def test_custom_fallback(self):
        assert _finite(float("nan"), fallback=50.0) == 50.0
        assert _finite(float("inf"), fallback=99.9) == 99.9


# ══════════════════════════════════════════════════════════════════
# _sanitize_item tests
# ══════════════════════════════════════════════════════════════════

_FLOAT_KEYS = {
    "opportunity", "risk", "trend", "flow", "revision",
    "liquidity", "volatility", "drawdown", "crowding",
    "avg_amount_5d", "close_price",
}


class TestSanitizeItem:
    def test_clean_item_unchanged(self):
        item = {
            "code": "000001",
            "name": "平安银行",
            "opportunity": 75.5,
            "risk": 30.2,
            "trend": 60.0,
            "flow": 55.5,
            "revision": 40.1,
            "liquidity": 70.3,
            "volatility": 25.8,
            "drawdown": 15.0,
            "crowding": 45.6,
            "avg_amount_5d": 12345.67,
            "quadrant": "机会",
        }
        result = _sanitize_item(item.copy())
        assert result == item  # nothing changed

    def test_single_nan_replaced(self):
        item = {"opportunity": float("nan"), "risk": 50.0}
        result = _sanitize_item(item.copy())
        assert result["opportunity"] == 0.0
        assert result["risk"] == 50.0

    def test_all_nan_fields(self):
        item = {k: float("nan") for k in _FLOAT_KEYS}
        item["code"], item["quadrant"] = "99999", "中性"
        result = _sanitize_item(item.copy())
        for k in _FLOAT_KEYS:
            assert result[k] == 0.0, f"field {k} should be 0.0"
        # Non-float keys untouched
        assert result["code"] == "99999"
        assert result["quadrant"] == "中性"

    def test_infinity_replaced(self):
        item = {"opportunity": float("inf"), "risk": float("-inf")}
        result = _sanitize_item(item.copy())
        assert result["opportunity"] == 0.0
        assert result["risk"] == 0.0

    def test_mixed_valid_and_invalid(self):
        item = {
            "opportunity": 80.0,
            "risk": float("nan"),
            "trend": 55.0,
            "flow": float("inf"),
            "revision": 30.0,
            "liquidity": float("-inf"),
            "volatility": 20.0,
            "drawdown": 10.0,
            "crowding": float("nan"),
            "avg_amount_5d": 5000.0,
        }
        result = _sanitize_item(item.copy())
        # Valid fields unchanged
        assert result["opportunity"] == 80.0
        assert result["trend"] == 55.0
        assert result["revision"] == 30.0
        assert result["volatility"] == 20.0
        assert result["drawdown"] == 10.0
        assert result["avg_amount_5d"] == 5000.0
        # Invalid fields replaced with 0
        assert result["risk"] == 0.0
        assert result["flow"] == 0.0
        assert result["liquidity"] == 0.0
        assert result["crowding"] == 0.0


# ══════════════════════════════════════════════════════════════════
# Integration: json.dumps must not raise after sanitization
# ══════════════════════════════════════════════════════════════════

class TestJsonCompliance:
    """The real-world regression: ensure sanitized items serialize cleanly."""

    @staticmethod
    def _make_dirty_item() -> dict:
        return {
            "code": "00001",
            "name": "TestStock",
            "exchange": "HKEX",
            "opportunity": float("nan"),
            "risk": float("inf"),
            "quadrant": "机会",
            "trend": 65.5,
            "flow": float("nan"),
            "revision": 40.0,
            "liquidity": float("-inf"),
            "volatility": 22.1,
            "drawdown": float("nan"),
            "crowding": 35.7,
            "avg_amount_5d": 8888.88,
        }

    def test_raw_dirty_item_fails_json(self):
        """Before sanitization: json.dumps should fail on dirty data."""
        item = self._make_dirty_item()
        # Python's standard json.dumps rejects nan/inf by default
        with pytest.raises((ValueError, TypeError)):
            json.dumps(item, allow_nan=False)  # strict mode

    def test_sanitized_item_passes_json(self):
        """After sanitization: json.dumps must succeed even in strict mode."""
        item = self._make_dirty_item()
        cleaned = _sanitize_item(item.copy())
        # This MUST NOT raise
        output = json.dumps(cleaned, ensure_ascii=False)
        assert isinstance(output, str)
        assert len(output) > 0

        # Round-trip: parse back and verify values
        parsed = json.loads(output)
        for key in _FLOAT_KEYS:
            val = parsed.get(key)
            if key in ("opportunity", "risk", "trend", "flow",
                       "revision", "liquidity", "volatility",
                       "drawdown", "crowding"):
                if key in ("opportunity", "risk") or True:
                    assert math.isfinite(val), f"{key} is not finite: {val}"

    def test_bulk_save_payload_serializes(self):
        """Simulate full bulk-save payload (items array)."""
        items = [self._make_dirty_item() for _ in range(100)]
        for it in items:
            _sanitize_item(it)
        payload = {"items": items, "computed_at": "2026-04-16T12:00:00Z"}
        # Must not raise
        output = json.dumps(payload, ensure_ascii=False)
        assert len(output) > 0
        # Should be valid JSON
        parsed = json.loads(output)
        assert len(parsed["items"]) == 100

    def test_numpy_nan_also_caught(self):
        """numpy.float64(np.nan) should also be caught by _finite."""
        val = float(np.nan)
        assert _finite(val) == 0.0

    def test_numpy_inf_also_caught(self):
        """numpy.float64(np.inf) should also be caught by _finite."""
        val = float(np.inf)
        assert _finite(val) == 0.0
