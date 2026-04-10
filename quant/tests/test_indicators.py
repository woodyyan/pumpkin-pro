"""Tests for technical indicator calculations (pure math, no external deps)."""

import numpy as np
import pandas as pd
import pytest
from indicators.technical_indicators import TechnicalIndicators


def _make_simple_data():
    """Create a minimal deterministic DataFrame for indicator tests."""
    np.random.seed(123)
    n = 100
    dates = pd.bdate_range("2025-06-01", periods=n)
    close = 50 + np.cumsum(np.random.randn(n) * 0.3)
    return pd.DataFrame({
        "date": dates,
        "open": close - 0.2,
        "high": close + abs(np.random.randn(n) * 0.3),
        "low": close - abs(np.random.randn(n) * 0.3),
        "close": close,
        "volume": np.random.randint(500000, 2000000, n).astype(float),
    })


class TestMovingAverage:
    """Test MA calculation."""

    @pytest.fixture
    def data(self):
        return _make_simple_data()

    @pytest.fixture
    def indicators(self, data):
        return TechnicalIndicators(data)

    def test_ma20_exists(self, indicators):
        ma = indicators.calculate_ma(20)
        assert len(ma) == len(indicators.data)
        # First 19 values should be NaN
        assert ma.iloc[:19].isna().all()
        # Value at index 19 should be finite
        assert not np.isnan(ma.iloc[19])

    def test_ma60_exists(self, indicators):
        ma = indicators.calculate_ma(60)
        assert len(ma) == len(indicators.data)
        assert ma.iloc[:59].isna().any()

    def test_ma_monotonic_convergence(self, indicators):
        """MA should smooth data — its std should be less than raw close."""
        ma20 = indicators.calculate_ma(20).dropna()
        raw = indicators.data["close"].iloc[19:]
        assert ma20.std() <= raw.std() + 1e-10


class TestRSI:
    """Test RSI calculation."""

    @pytest.fixture
    def indicators(self):
        return TechnicalIndicators(_make_simple_data())

    def test_rsi_range(self, indicators):
        rsi = indicators.calculate_rsi(period=14)
        valid = rsi.dropna()
        # RSI should be between 0 and 100
        assert (valid >= 0).all() or (valid.isna()).all()
        assert (valid <= 100).all() or (valid.isna()).all()

    def test_rsi_length(self, indicators):
        rsi = indicators.calculate_rsi(period=14)
        assert len(rsi) == len(indicators.data)


class TestATR:
    """Test ATR calculation."""

    @pytest.fixture
    def indicators(self):
        return TechnicalIndicators(_make_simple_data())

    def test_atr_positive(self, indicators):
        atr = indicators.calculate_atr(period=14)
        valid = atr.dropna()
        assert (valid >= 0).all()

    def test_atr_length(self, indicators):
        atr = indicators.calculate_atr(period=14)
        assert len(atr) == len(indicators.data)


class TestBollingerBands:
    """Test Bollinger Bands calculation."""

    @pytest.fixture
    def indicators(self):
        return TechnicalIndicators(_make_simple_data())

    def test_bollinger_bands_structure(self, indicators):
        upper, mid, lower = indicators.calculate_bollinger_bands(period=20, std_dev=2.0)
        assert len(upper) == len(mid) == len(lower) == len(indicators.data)

    def test_bollinger_ordering(self, indicators):
        upper, mid, lower = indicators.calculate_bollinger_bands(period=20, std_dev=2.0)
        # Upper should generally be >= mid >= lower (after warmup)
        valid_idx = mid.dropna().index
        for i in valid_idx[5:]:
            assert upper[i] >= mid[i] - 1e-10, f"upper[{i}]={upper[i]} < mid[{i}]={mid[i]}"
            assert lower[i] <= mid[i] + 1e-10, f"lower[{i}]={lower[i]} > mid[{i}]={mid[i]}"


class TestMACD:
    """Test MACD calculation."""

    @pytest.fixture
    def indicators(self):
        return TechnicalIndicators(_make_simple_data())

    def test_macd_components(self, indicators):
        dif, dea, hist = indicators.calculate_macd()
        assert len(dif) == len(dea) == len(hist) == len(indicators.data)

    def test_histogram_is_difference(self, indicators):
        dif, dea, _ = indicators.calculate_macd()
        expected_hist = (dif - dea) * 2
        _, _, actual_hist = indicators.calculate_macd()
        # Allow tiny float tolerance
        diff = (expected_hist - actual_hist).dropna().abs()
        assert (diff < 1e-10).all(), f"MACD histogram mismatch: max diff = {diff.max()}"
