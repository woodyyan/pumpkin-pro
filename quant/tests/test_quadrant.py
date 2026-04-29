import numpy as np
import pandas as pd
import pytest

pytest.importorskip("requests", reason="screener.quadrant 依赖 requests")

from screener.quadrant import _classify_a_share_board, _compute_daily_metrics, _safe_momentum


def test_classify_a_share_board():
    assert _classify_a_share_board("600519") == "MAIN"
    assert _classify_a_share_board("000001") == "MAIN"
    assert _classify_a_share_board("002415") == "MAIN"
    assert _classify_a_share_board("300750") == "CHINEXT"
    assert _classify_a_share_board("301123") == "CHINEXT"
    assert _classify_a_share_board("688981") == "STAR"
    assert _classify_a_share_board("689009") == "STAR"
    assert _classify_a_share_board("430047") == "OTHER"


def test_safe_momentum_prefers_stable_stock():
    volatile = pd.Series([20.0], index=["A"])
    stable = pd.Series([15.0], index=["B"])
    returns = pd.concat([volatile, stable])
    volatility = pd.Series([0.20, 0.05], index=["A", "B"])

    momentum = _safe_momentum(returns, volatility)

    assert momentum["B"] > momentum["A"]


def test_safe_momentum_uses_floor_for_zero_or_missing_volatility():
    returns = pd.Series([12.5, 8.0], index=["zero", "nan"])
    volatility = pd.Series([0.0, np.nan], index=["zero", "nan"])

    momentum = _safe_momentum(returns, volatility, floor=0.005)

    assert momentum["zero"] == pytest.approx(2500.0)
    assert momentum["nan"] == pytest.approx(1600.0)


def test_compute_daily_metrics_includes_std_60d():
    close = pd.Series([100 + i * 0.8 + ((-1) ** i) * 0.5 for i in range(70)], dtype=float)
    volume = pd.Series([1_000_000 + i * 10_000 for i in range(70)], dtype=float)
    daily_df = pd.DataFrame(
        {
            "date": pd.date_range("2026-01-01", periods=70, freq="D"),
            "close": close,
            "volume": volume,
        }
    )

    metrics = _compute_daily_metrics(daily_df)
    expected_std_60d = float(close.pct_change().dropna().tail(60).std())

    assert metrics["std_20d"] > 0
    assert metrics["std_60d"] == pytest.approx(expected_std_60d)
    assert metrics["avg_amount_5d"] > 0


def test_compute_daily_metrics_std_60d_falls_back_to_full_window_when_under_60_days():
    close = pd.Series([50 + i * 0.6 + (i % 3) * 0.2 for i in range(35)], dtype=float)
    daily_df = pd.DataFrame(
        {
            "date": pd.date_range("2026-02-01", periods=35, freq="D"),
            "close": close,
            "volume": pd.Series([500_000 + i * 5000 for i in range(35)], dtype=float),
        }
    )

    metrics = _compute_daily_metrics(daily_df)
    expected_std = float(close.pct_change().dropna().std())

    assert metrics["std_60d"] == pytest.approx(expected_std)
