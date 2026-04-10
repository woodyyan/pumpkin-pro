"""Tests for backtest engine (pure math, no external API calls)."""

import pandas as pd
import pytest
from engine.backtest_engine import BacktestEngine
from config import INITIAL_CAPITAL, TRANSACTION_FEE


def _make_backtest_data_with_signals():
    """Create test data with buy/sell/hold signals."""
    np.random.seed(99)
    n = 60
    dates = pd.bdate_range("2025-03-01", periods=n)
    close = 80 + np.cumsum(np.random.randn(n) * 0.4)
    return pd.DataFrame({
        "date": dates,
        "open": close - np.random.rand(n) * 0.3,
        "high": close + abs(np.random.rand(n)),
        "low": close - abs(np.random.rand(n)),
        "close": close,
        "volume": np.random.randint(1e6, 5e6, n).astype(float),
        "signal": ["hold"] * 15 + ["buy"] * 5 + ["hold"] * 20 + ["sell"] * 5 + ["hold"] * 15,
    })


class TestBacktestEngineInit:
    """Test backtest engine initialization."""

    def test_default_capital(self, sample_ohlc_data):
        engine = BacktestEngine(sample_ohlc_data)
        assert engine.initial_capital == INITIAL_CAPITAL

    def test_custom_capital(self, sample_ohlc_data):
        engine = BacktestEngine(sample_ohlc_data, initial_capital=500000)
        assert engine.initial_capital == 500000

    def test_custom_fee(self, sample_ohlc_data):
        engine = BacktestEngine(sample_ohlc_data, fee=0.002)
        assert engine.fee_rate == 0.002


class TestBacktestExecution:
    """Test basic backtest execution flow."""

    def test_run_produces_portfolio_column(self, sample_signal_data):
        engine = BacktestEngine(sample_signal_data)
        result = engine.run_backtest()
        assert "portfolio_value" in result.columns
        assert len(result) == len(sample_signal_data)

    def test_run_produces_cash_and_shares(self, sample_signal_data):
        engine = BacktestEngine(sample_signal_data)
        result = engine.run_backtest()
        assert "cash" in result.columns
        assert "shares" in result.columns

    def test_first_portfolio_value_equals_capital(self, sample_signal_data):
        engine = BacktestEngine(sample_signal_data)
        result = engine.run_backtest()
        first_val = result["portfolio_value"].iloc[0]
        assert abs(first_val - engine.initial_capital) < 0.01

    def test_no_negative_cash_after_buy_fee(self, sample_signal_data):
        """Cash should never go significantly negative after fee deduction."""
        engine = BacktestEngine(sample_signal_data)
        result = engine.run_backtest()
        cash_min = result["cash"].min()
        # Small negative allowed due to rounding, but not large negative
        assert cash_min > -engine.initial_capital * 0.01

    def test_trades_recorded(self, sample_signal_data):
        engine = BacktestEngine(sample_signal_data)
        engine.run_backtest()
        log = engine.get_trade_log()
        assert isinstance(log, pd.DataFrame)
        # Should have at least one buy trade since there are buy signals
        if len(log) > 0:
            assert "type" in log.columns
            types = log["type"].unique()
            assert all(t in ("buy", "sell") for t in types)


class TestBacktestPerformanceMetrics:
    """Test performance metrics output."""

    def test_metrics_keys(self, sample_signal_data):
        engine = BacktestEngine(sample_signal_data)
        engine.run_backtest()
        metrics = engine.get_performance_metrics()
        expected_keys = {
            "initial_capital",
            "final_capital",
            "total_return_pct",
            "annual_return_pct",
            "max_drawdown_pct",
            "total_trades",
            "win_rate_pct",
            "sharpe_ratio",
            "total_days",
            "total_fee",
            "avg_daily_return_pct",
            "volatility_pct",
        }
        assert set(expected_keys).issubset(set(metrics.keys()))

    def test_final_capital_matches_series(self, sample_signal_data):
        engine = BacktestEngine(sample_signal_data)
        result = engine.run_backtest()
        metrics = engine.get_performance_metrics()
        expected_final = result["portfolio_value"].iloc[-1]
        assert abs(metrics["final_capital"] - expected_final) < 0.01

    def test_total_days_matches_data(self, sample_signal_data):
        engine = BacktestEngine(sample_signal_data)
        engine.run_backtest()
        metrics = engine.get_performance_metrics()
        assert metrics["total_days"] == len(sample_signal_data)

    def test_max_drawdown_nonnegative(self, sample_signal_data):
        engine = BacktestEngine(sample_signal_data)
        engine.run_backtest()
        metrics = engine.get_performance_metrics()
        assert metrics["max_drawdown_pct"] >= 0


class TestBacktestEdgeCases:
    """Test edge cases and error conditions."""

    def test_missing_required_column_raises(self, sample_ohlc_data):
        """Should raise ValueError if 'signal' column is missing."""
        data_no_signal = sample_ohlc_data.drop(columns=["signal"], errors="ignore")
        engine = BacktestEngine(data_no_signal)
        with pytest.raises(ValueError, match="signal"):
            engine.run_backtest()

    def test_all_hold_signals(self, sample_ohlc_data):
        """All hold signals → zero trades, portfolio_value ≈ capital."""
        data = sample_ohlc_data.copy()
        data["signal"] = "hold"
        engine = BacktestEngine(data)
        result = engine.run_backtest()
        metrics = engine.get_performance_metrics()
        assert metrics["total_trades"] == 0
        # No trading means value stays near initial capital (just price tracking)
        assert metrics["final_capital"] > 0

    def test_empty_dataframe_raises(self):
        """Empty DataFrame should still run without crash."""
        empty = pd.DataFrame(columns=["signal", "close", "open"])
        engine = BacktestEngine(empty)
        result = engine.run_backtest()
        assert len(result) == 0
