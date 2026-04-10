"""Shared fixtures for Quant tests."""

import numpy as np
import pandas as pd
import pytest


@pytest.fixture
def sample_ohlc_data():
    """Generate standard OHLCV test data (120 trading days).

    Uses a seeded random walk so results are deterministic across runs.
    """
    np.random.seed(42)
    n = 120
    dates = pd.bdate_range("2025-01-01", periods=n)
    price = 100 + np.cumsum(np.random.randn(n) * 0.5)
    return pd.DataFrame({
        "date": dates,
        "open": price - np.random.rand(n) * 0.5,
        "high": price + abs(np.random.rand(n)),
        "low": price - abs(np.random.rand(n)),
        "close": price,
        "volume": np.random.randint(1e6, 1e7, n).astype(float),
    })


@pytest.fixture
def sample_signal_data(sample_ohlc_data):
    """Add signal column to OHLC data using simple MA20 crossover strategy."""
    df = sample_ohlc_data.copy()
    ma20 = df["close"].rolling(window=20, min_periods=20).mean()
    df["signal"] = "hold"
    # Buy when close > MA20
    df.loc[df["close"] > ma20, "signal"] = "buy"
    # Sell when previous close was above MA20 but current is not (simplified)
    above_prev = df["close"].shift(1) > ma20.shift(1)
    below_now = df["close"] <= ma20
    df.loc[above_prev & below_now, "signal"] = "sell"
    # First 19 rows have no MA20 — set to hold
    df.iloc[:19, df.columns.get_loc("signal")] = "hold"
    return df
