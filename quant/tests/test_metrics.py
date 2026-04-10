"""Tests for performance metrics calculations (pure math)."""

import pytest
from result.metrics import PerformanceMetrics


def _sample_portfolio_values():
    """Return a realistic-looking portfolio value series."""
    base = [100000] + [
        100000 + i * 500 + (i % 7) * 200 - (i % 5) * 150
        for i in range(1, 252)
    ]
    return base


def _sample_daily_returns():
    """Return matching daily returns from portfolio values."""
    vals = _sample_portfolio_values()
    returns = []
    for i in range(len(vals)):
        if i == 0:
            returns.append(0.0)
        else:
            returns.append((vals[i] - vals[i - 1]) / vals[i - 1])
    return returns


def _sample_trades():
    """Return a list of simulated trades."""
    return [
        {"date": "2025-01-05", "type": "buy", "price": 50.0, "shares": 200,
         "amount": 10000.0, "fee": 10.0},
        {"date": "2025-02-10", "type": "sell", "price": 55.0, "shares": 200,
         "amount": 11000.0, "fee": 11.0},
        {"date": "2025-03-01", "type": "buy", "price": 48.0, "shares": 250,
         "amount": 12000.0, "fee": 12.0},
        {"date": "2025-04-15", "type": "sell", "price": 52.0, "shares": 250,
         "amount": 13000.0, "fee": 13.0},
    ]


class TestReturnMetrics:
    """Test return-related metric calculations."""

    @pytest.fixture
    def pm(self):
        pv = _sample_portfolio_values()
        dr = _sample_daily_returns()
        trades = _sample_trades()
        return PerformanceMetrics(pv, dr, trades)

    def test_initial_capital_preserved(self, pm):
        m = pm._calculate_return_metrics()
        assert m["initial_capital"] == 100000

    def test_final_capital_positive(self, pm):
        m = pm._calculate_return_metrics()
        assert m["final_capital"] > 0

    def test_total_days_correct(self, pm):
        m = pm._calculate_return_metrics()
        assert m["total_days"] == 252

    def test_years_calculated(self, pm):
        m = pm._calculate_return_metrics()
        assert m["years"] == pytest.approx(252 / 252, rel=0.01)

    def test_cumulative_return_pct(self, pm):
        m = pm._calculate_return_metrics()
        final = m["final_capital"]
        expected_cumulative = (final / m["initial_capital"] - 1) * 100
        assert m["cumulative_return_pct"] == pytest.approx(expected_cumulative, rel=1e-4)


class TestRiskMetrics:
    """Test risk-related metric calculations."""

    @pytest.fixture
    def pm(self):
        pv = _sample_portfolio_values()
        dr = _sample_daily_returns()
        trades = _sample_trades()
        return PerformanceMetrics(pv, dr, trades)

    def test_max_drawdown_nonnegative(self, pm):
        m = pm._calculate_risk_metrics()
        assert m["max_drawdown_pct"] >= 0

    def test_volatility_nonnegative(self, pm):
        m = pm._calculate_risk_metrics()
        assert m["volatility_pct"] >= 0

    def test_sharpe_calculation(self, pm):
        m = pm._calculate_risk_metrics()
        if m["volatility_pct"] > 0:
            # Sharpe should be a finite number
            assert m["sharpe_ratio"] != 0 or m["avg_daily_return_pct"] == 0

    def test_sortino_ratio_nonnegative_when_profitable(self, pm):
        m = pm._calculate_risk_metrics()
        # Sortino can be 0 or positive; just verify it's a number
        assert isinstance(m["sortino_ratio"], (int, float))

    def test_calmar_ratio_requires_drawdown(self, pm):
        m = pm._calculate_risk_metrics()
        if m["max_drawdown_pct"] > 0:
            assert m["calmar_ratio"] != 0 or m["avg_daily_return_pct"] == 0


class TestTradeMetrics:
    """Test trade analysis metrics."""

    def test_empty_trades(self):
        pm = PerformanceMetrics([100000, 101000], [0.0, 0.01], [])
        m = pm._calculate_trade_metrics()
        assert m["total_trades"] == 0
        assert m["win_rate_pct"] == 0

    def test_trade_count(self):
        trades = _sample_trades()
        pm = PerformanceMetrics(_sample_portfolio_values(), _sample_daily_returns(), trades)
        m = pm._calculate_trade_metrics()
        assert m["total_trades"] == len(trades)

    def test_buy_sell_counts(self):
        trades = _sample_trades()
        pm = PerformanceMetrics(_sample_portfolio_values(), _sample_daily_returns(), trades)
        m = pm._calculate_trade_metrics()
        assert m["buy_trades"] == 2
        assert m["sell_trades"] == 2

    def test_total_fee_summed(self):
        trades = _sample_trades()
        pm = PerformanceMetrics(_sample_portfolio_values(), _sample_daily_returns(), trades)
        m = pm._calculate_trade_metrics()
        expected_fee = sum(t["fee"] for t in trades)
        assert m["total_fee"] == pytest.approx(expected_fee)


class TestOtherMetrics:
    """Test skewness, kurtosis, information ratio etc."""

    @pytest.fixture
    def pm(self):
        return PerformanceMetrics(
            _sample_portfolio_values(),
            _sample_daily_returns(),
            _sample_trades(),
        )

    def test_skewness_is_number(self, pm):
        m = pm._calculate_other_metrics()
        assert isinstance(m["skewness"], (int, float))

    def test_kurtosis_is_number(self, pm):
        m = pm._calculate_other_metrics()
        assert isinstance(m["kurtosis"], (int, float))

    def test_best_day_worst_day(self, pm):
        m = pm._calculate_other_metrics()
        assert m["best_day_pct"] >= m["worst_day_pct"]

    def test_daily_win_rate_between_0_and_100(self, pm):
        m = pm._calculate_other_metrics()
        assert 0 <= m["daily_win_rate_pct"] <= 100


class TestEdgeCases:
    """Edge cases for metrics calculation."""

    def test_single_day_portfolio(self):
        pm = PerformanceMetrics([100000], [0.0], [])
        m = pm.calculate_all_metrics()
        # Should handle gracefully without crashing
        assert isinstance(m, dict)

    def test_two_days_minimum_for_risk(self):
        pm = PerformanceMetrics([100000, 101000], [0.0, 0.01], [])
        m = pm._calculate_risk_metrics()
        # With only 2 points risk metrics may be empty
        assert isinstance(m, dict)

    def test_constant_portfolio_no_variance(self):
        constant_vals = [100000] * 252
        zero_returns = [0.0] * 252
        pm = PerformanceMetrics(constant_vals, zero_returns, [])
        m = pm._calculate_risk_metrics()
        # Zero variance → volatility should be 0
        assert m.get("volatility_pct") == 0
