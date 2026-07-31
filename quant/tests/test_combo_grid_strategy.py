"""
Tests for the combo_grid strategy: signal projection, indicator attach,
registry validation, and an end-to-end backtest smoke through BacktestEngine.
No network / akshare required — uses synthetic OHLCV fixtures.
"""
import numpy as np
import pandas as pd
import pytest


# ──────────────────────────────────────────────
# Fixtures
# ──────────────────────────────────────────────

DEFAULT_PARAMS = {
    "box_lookback": 20,
    "box_method": "minmax",
    "grid_num": 8,
    "grid_capital_ratio": 0.60,
    "grid_stop_pct": 0.025,
    "rsi_window": 14,
    "rsi_oversold": 30,
    "rsi_overbought": 70,
    "boll_window": 20,
    "boll_std": 2.0,
    "timing_capital_ratio": 0.30,
    "vol_multiple": 1.5,
    "break_buffer": 0.0,
    "trail_stop_pct": 0.043,
    "break_capital_ratio": 0.10,
}


@pytest.fixture()
def ohlcv_120():
    """120-row oscillating OHLCV — enough to warm up a 20-day box + boll + rsi."""
    np.random.seed(7)
    n = 120
    idx = pd.date_range("2024-01-02", periods=n, freq="B")
    # oscillating base to exercise grid buys/sells, plus a couple of spikes
    t = np.arange(n)
    close = 100.0 + 8.0 * np.sin(t / 6.0) + np.cumsum(np.random.randn(n) * 0.2)
    high = close + abs(np.random.randn(n)) * 0.6
    low = close - abs(np.random.randn(n)) * 0.6
    open_ = close + np.random.randn(n) * 0.3
    volume = np.random.randint(1000, 5000, n).astype(float)
    # inject a volume+price spike near the end to trigger breakout
    high[110] = close[110] * 1.20
    close[110] = close[110] * 1.18
    volume[110] = volume.mean() * 4
    return pd.DataFrame(
        {"date": idx, "open": open_, "high": high, "low": low, "close": close, "volume": volume},
        index=idx,
    )


def _attach(df, params=None):
    from strategy_library.registry import StrategyRegistry

    p = dict(DEFAULT_PARAMS)
    if params:
        p.update(params)
    adapter = StrategyRegistry().get_adapter("combo_grid")
    return adapter.attach_indicators(df, p), p


# ════════════════════════════════════════════
# Strategy signal generation
# ════════════════════════════════════════════

class TestComboGridStrategy:
    def test_generate_signals_creates_columns(self, ohlcv_120):
        from strategy.combo_grid_strategy import ComboGridStrategy

        enriched, p = _attach(ohlcv_120)
        result = ComboGridStrategy(
            enriched,
            grid_num=p["grid_num"],
            grid_capital_ratio=p["grid_capital_ratio"],
            grid_stop_pct=p["grid_stop_pct"],
            rsi_oversold=p["rsi_oversold"],
            rsi_overbought=p["rsi_overbought"],
            timing_capital_ratio=p["timing_capital_ratio"],
            vol_multiple=p["vol_multiple"],
            trail_stop_pct=p["trail_stop_pct"],
            break_capital_ratio=p["break_capital_ratio"],
        ).generate_signals()
        assert "signal" in result.columns
        assert "signal_size" in result.columns
        assert "combo_leg" in result.columns

    def test_all_signals_valid_and_sizes_bounded(self, ohlcv_120):
        from strategy.combo_grid_strategy import ComboGridStrategy

        enriched, p = _attach(ohlcv_120)
        result = ComboGridStrategy(enriched, **_decision_kwargs(p)).generate_signals()
        assert set(result["signal"]).issubset({"buy", "sell", "hold"})
        # size must be within [0, 1]
        assert (result["signal_size"] >= 0).all()
        assert (result["signal_size"] <= 1.0 + 1e-9).all()
        # hold rows carry size 0
        holds = result[result["signal"] == "hold"]
        assert (holds["signal_size"] == 0).all()

    def test_warmup_rows_are_hold(self, ohlcv_120):
        from strategy.combo_grid_strategy import ComboGridStrategy

        enriched, p = _attach(ohlcv_120)
        result = ComboGridStrategy(enriched, **_decision_kwargs(p)).generate_signals()
        # first (box_lookback - 1) rows have NaN box → must be hold
        assert (result["signal"].iloc[: p["box_lookback"] - 1] == "hold").all()

    def test_missing_indicator_columns_raises(self, ohlcv_120):
        from strategy.combo_grid_strategy import ComboGridStrategy

        with pytest.raises(ValueError, match="缺失指标"):
            ComboGridStrategy(ohlcv_120, **_decision_kwargs(DEFAULT_PARAMS)).generate_signals()

    def test_empty_dataframe(self):
        from strategy.combo_grid_strategy import ComboGridStrategy

        df = pd.DataFrame(columns=["open", "high", "low", "close", "volume"])
        result = ComboGridStrategy(df, **_decision_kwargs(DEFAULT_PARAMS)).generate_signals()
        assert len(result) == 0

    def test_breakout_leg_is_recorded(self, ohlcv_120):
        from strategy.combo_grid_strategy import ComboGridStrategy

        enriched, p = _attach(ohlcv_120)
        result = ComboGridStrategy(enriched, **_decision_kwargs(p)).generate_signals()
        legs = set(str(x) for x in result["combo_leg"] if str(x))
        # the injected spike should surface at least one non-empty leg label
        assert len(legs) >= 1


def _decision_kwargs(p):
    return dict(
        grid_num=p["grid_num"],
        grid_capital_ratio=p["grid_capital_ratio"],
        grid_stop_pct=p["grid_stop_pct"],
        rsi_oversold=p["rsi_oversold"],
        rsi_overbought=p["rsi_overbought"],
        timing_capital_ratio=p["timing_capital_ratio"],
        break_buffer=p.get("break_buffer", 0.0),
        vol_multiple=p["vol_multiple"],
        trail_stop_pct=p["trail_stop_pct"],
        break_capital_ratio=p["break_capital_ratio"],
    )


# ════════════════════════════════════════════
# Indicator attach
# ════════════════════════════════════════════

class TestComboGridAttachIndicators:
    def test_attaches_all_required_columns(self, ohlcv_120):
        enriched, _ = _attach(ohlcv_120)
        for col in [
            "combo_box_up", "combo_box_low", "combo_grid_step", "combo_stop_line",
            "combo_break_up_line", "combo_break_down_line", "combo_boll_up",
            "combo_boll_low", "combo_rsi", "combo_vol_ma",
        ]:
            assert col in enriched.columns

    def test_box_method_boll(self, ohlcv_120):
        enriched, _ = _attach(ohlcv_120, {"box_method": "boll"})
        assert "combo_box_up" in enriched.columns
        assert enriched["combo_box_up"].notna().any()

    def test_box_method_percentile(self, ohlcv_120):
        enriched, _ = _attach(ohlcv_120, {"box_method": "percentile"})
        assert enriched["combo_box_up"].notna().any()


# ════════════════════════════════════════════
# Registry validation
# ════════════════════════════════════════════

class TestComboGridValidation:
    def _adapter(self):
        from strategy_library.registry import StrategyRegistry

        return StrategyRegistry().get_adapter("combo_grid")

    def test_valid_params_pass(self):
        self._adapter().validate_params(dict(DEFAULT_PARAMS))

    def test_ratios_must_sum_to_one(self):
        bad = dict(DEFAULT_PARAMS, grid_capital_ratio=0.5, timing_capital_ratio=0.3, break_capital_ratio=0.3)
        with pytest.raises(ValueError, match="资金比例之和必须等于 1"):
            self._adapter().validate_params(bad)

    def test_grid_num_min(self):
        with pytest.raises(ValueError, match="网格档数最小为 2"):
            self._adapter().validate_params(dict(DEFAULT_PARAMS, grid_num=1))

    def test_rsi_thresholds(self):
        with pytest.raises(ValueError, match="RSI 超卖阈值必须小于超买阈值"):
            self._adapter().validate_params(dict(DEFAULT_PARAMS, rsi_oversold=70, rsi_overbought=30))

    def test_invalid_box_method(self):
        with pytest.raises(ValueError, match="箱体方式必须为"):
            self._adapter().validate_params(dict(DEFAULT_PARAMS, box_method="atr"))

    def test_overlay_columns(self):
        cols = self._adapter().get_overlay_columns(dict(DEFAULT_PARAMS))
        assert cols == ["combo_box_up", "combo_box_low", "combo_boll_up", "combo_boll_low"]


# ════════════════════════════════════════════
# End-to-end backtest smoke (attach → build → signals → engine)
# ════════════════════════════════════════════

class TestComboGridBacktestSmoke:
    def test_backtest_runs_and_produces_curve(self, ohlcv_120):
        from strategy_library.registry import StrategyRegistry
        from engine.backtest_engine import BacktestEngine

        adapter = StrategyRegistry().get_adapter("combo_grid")
        p = dict(DEFAULT_PARAMS)
        adapter.validate_params(p)
        enriched = adapter.attach_indicators(ohlcv_120, p)
        strat = adapter.build_strategy(enriched, p)
        data_with_signals = strat.generate_signals()

        engine = BacktestEngine(data_with_signals, initial_capital=1_000_000, fee=0.0012)
        result = engine.run_backtest()

        assert "portfolio_value" in result.columns
        assert "cumulative_return" in result.columns
        assert len(result) == len(ohlcv_120)
        # portfolio value must stay finite and non-negative
        assert result["portfolio_value"].notna().all()
        assert (result["portfolio_value"] >= 0).all()
