"""
Tests for all 8 strategy modules + strategy_library registry/service.
Uses sample DataFrame fixtures — no akshare/network required.
"""
import pytest
import pandas as pd
import numpy as np


# ──────────────────────────────────────────────
# Fixtures
# ──────────────────────────────────────────────

@pytest.fixture()
def ohlcv_50():
    """50-row OHLCV DataFrame with date index."""
    np.random.seed(42)
    n = 50
    idx = pd.date_range("2024-01-02", periods=n, freq="B")
    close = 100.0 + np.cumsum(np.random.randn(n) * 0.5)
    return pd.DataFrame(
        {
            "date": idx,
            "open": close + np.random.randn(n) * 0.3,
            "high": close + abs(np.random.randn(n)) * 0.5,
            "low": close - abs(np.random.randn(n)) * 0.5,
            "close": close,
            "volume": np.random.randint(1000, 10000, n),
        },
        index=idx,
    )


def _enrich_with_mas(df, short=10, long=30):
    df = df.copy()
    df[f"MA{short}"] = df["close"].rolling(window=short, min_periods=short).mean()
    df[f"MA{long}"] = df["close"].rolling(window=long, min_periods=long).mean()
    return df


def _enrich_with_macd(df, fast=12, slow=26, signal=9):
    from indicators.technical_indicators import TechnicalIndicators

    calc = TechnicalIndicators(df)
    enriched = calc.data.copy()
    dif, dea, hist = calc.calculate_macd(fast_period=fast, slow_period=slow, signal_period=signal)
    enriched["MACD_DIF"] = dif
    enriched["MACD_DEA"] = dea
    enriched["MACD_HIST"] = hist
    return enriched


def _enrich_with_bb(df, period=20, std=2.0):
    from indicators.technical_indicators import TechnicalIndicators

    calc = TechnicalIndicators(df)
    enriched = calc.data.copy()
    upper, mid, lower = calc.calculate_bollinger_bands(period=int(period), std_dev=float(std))
    enriched["BB_upper"] = upper
    enriched["BB_mid"] = mid
    enriched["BB_lower"] = lower
    return enriched


def _enrich_with_rsi(df, period=14):
    from indicators.technical_indicators import TechnicalIndicators

    calc = TechnicalIndicators(df)
    enriched = calc.data.copy()
    enriched[f"RSI_{period}"] = calc.calculate_rsi(period=period)
    return enriched


def _enrich_volume_breakout(df, lookback=20, exit_ma=20):
    df = df.copy()
    lb = int(lookback)
    ema = int(exit_ma)
    from indicators.technical_indicators import TechnicalIndicators

    calc = TechnicalIndicators(df)
    enriched = calc.data.copy()
    enriched[f"VOL_MA{lb}"] = enriched["volume"].rolling(window=lb, min_periods=lb).mean()
    enriched[f"HIGH_{lb}"] = enriched["high"].rolling(window=lb, min_periods=lb).max()
    enriched[f"MA{ema}"] = calc.calculate_ma(ema)
    return enriched


# ════════════════════════════════════════════
# MACD Strategy Tests
# ════════════════════════════════════════════

class TestMACDStrategy:
    def test_generate_signals_creates_columns(self, ohlcv_50):
        from strategy.macd_strategy import MACDStrategy

        data = _enrich_with_macd(ohlcv_50)
        result = MACDStrategy(data).generate_signals()

        assert "signal" in result.columns
        assert "signal_size" in result.columns
        assert result["signal"].iloc[0] == "hold"

    def test_missing_columns_raises(self, ohlcv_50):
        from strategy.macd_strategy import MACDStrategy

        with pytest.raises(ValueError, match="缺失 MACD"):
            MACDStrategy(ohlcv_50).generate_signals()

    def test_all_signals_are_valid(self, ohlcv_50):
        from strategy.macd_strategy import MACDStrategy

        data = _enrich_with_macd(ohlcv_50)
        result = MACDStrategy(data).generate_signals()
        valid = {"buy", "sell", "hold"}
        assert set(result["signal"]).issubset(valid)


# ════════════════════════════════════════════
# Trend Strategy Tests
# ════════════════════════════════════════════

class TestTrendStrategy:
    def test_generate_signals_basic(self, ohlcv_50):
        from strategy.trend_strategy import TrendStrategy

        data = _enrich_with_mas(ohlcv_50)
        result = TrendStrategy(data, ma_short=10, ma_long=30).generate_signals()
        assert "signal" in result.columns
        assert "position" in result.columns

    def test_missing_ma_columns_raises(self, ohlcv_50):
        from strategy.trend_strategy import TrendStrategy

        with pytest.raises(ValueError, match="缺少必要的均线指标"):
            TrendStrategy(ohlcv_50).generate_signals()

    def test_default_params_use_config(self, ohlcv_50):
        from strategy.trend_strategy import TrendStrategy
        from config import MA_SHORT, MA_LONG

        data = _enrich_with_mas(ohlcv_50, short=MA_SHORT, long=MA_LONG)
        result = TrendStrategy(data).generate_signals()
        assert "signal" in result.columns

    def test_signal_details_structure(self, ohlcv_50):
        from strategy.trend_strategy import TrendStrategy

        data = _enrich_with_mas(ohlcv_50)
        data["date"] = data.index
        strat = TrendStrategy(data, ma_short=10, ma_long=30)
        strat.generate_signals()
        details = strat.get_signal_details()
        assert "buy_signals" in details
        assert "sell_signals" in details
        assert "hold_periods" in details

    @pytest.mark.skip("SimpleStrategy depends on config MA_SHORT/MA_LONG defaults")
    def test_simple_strategy_static_method(self, ohlcv_50):
        pass


# ════════════════════════════════════════════
# Bollinger+MACD Strategy Tests
# ════════════════════════════════════════════

class TestBollingerMACDStrategy:
    def _make_data(self, ohlcv_50):
        data = _enrich_with_bb(ohlcv_50)
        data = _enrich_with_macd(data)
        return data

    def test_and_mode(self, ohlcv_50):
        from strategy.bollinger_macd_strategy import BollingerMACDStrategy

        data = self._make_data(ohlcv_50)
        result = BollingerMACDStrategy(data, logic_mode="and").generate_signals()
        assert "signal" in result.columns

    def test_or_mode(self, ohlcv_50):
        from strategy.bollinger_macd_strategy import BollingerMACDStrategy

        data = self._make_data(ohlcv_50)
        result = BollingerMACDStrategy(data, logic_mode="or").generate_signals()
        valid = {"buy", "sell", "hold"}
        assert set(result["signal"]).issubset(valid)

    def test_missing_columns_raises(self, ohlcv_50):
        from strategy.bollinger_macd_strategy import BollingerMACDStrategy

        with pytest.raises(ValueError, match="缺失指标"):
            BollingerMACDStrategy(ohlcv_50).generate_signals()


# ════════════════════════════════════════════
# Dual Confirm Strategy Tests
# ══════════════════════════════════════════

class TestDualConfirmStrategy:
    def _make_data(self, ohlcv_50):
        data = _enrich_with_mas(ohlcv_50, short=10, long=30)
        data = _enrich_with_rsi(data, period=14)
        return data

    def test_and_mode_generates_signals(self, ohlcv_50):
        from strategy.dual_confirm_strategy import DualConfirmStrategy

        data = self._make_data(ohlcv_50)
        result = DualConfirmStrategy(
            data, ma_short=10, ma_long=30, rsi_period=14,
            rsi_low=35, rsi_high=70, confirm_window=5, logic_mode="and",
        ).generate_signals()
        assert "signal" in result.columns

    def test_or_mode_generates_signals(self, ohlcv_50):
        from strategy.dual_confirm_strategy import DualConfirmStrategy

        data = self._make_data(ohlcv_50)
        result = DualConfirmStrategy(
            data, ma_short=10, ma_long=30, rsi_period=14,
            rsi_low=35, rsi_high=70, confirm_window=5, logic_mode="or",
        ).generate_signals()
        assert "signal" in result.columns

    def test_missing_column_raises(self, ohlcv_50):
        from strategy.dual_confirm_strategy import DualConfirmStrategy

        with pytest.raises(ValueError, match="缺失指标"):
            DualConfirmStrategy(ohlcv_50).generate_signals()


# ════════════════════════════════════════════
# Volume Breakout Strategy Tests
# ════════════════════════════════════════════

class TestVolumeBreakoutStrategy:
    def test_basic_generation(self, ohlcv_50):
        from strategy.volume_breakout_strategy import VolumeBreakoutStrategy

        data = _enrich_volume_breakout(ohlcv_50, lookback=20, exit_ma=20)
        result = VolumeBreakoutStrategy(
            data, lookback=20, volume_multiple=2.0, exit_ma_period=20,
        ).generate_signals()
        assert "signal" in result.columns

    def test_missing_vol_ma_raises(self, ohlcv_50):
        from strategy.volume_breakout_strategy import VolumeBreakoutStrategy

        with pytest.raises(ValueError, match="缺失均量指标"):
            VolumeBreakoutStrategy(ohlcv_50).generate_signals()


# ════════════════════════════════════════════
# Mean Reversion Strategy Tests
# ════════════════════════════════════════════

class TestMeanReversionStrategy:
    def test_basic_generation(self, ohlcv_50):
        from strategy.mean_reversion_strategy import MeanReversionStrategy

        data = _enrich_with_bb(ohlcv_50)
        result = MeanReversionStrategy(data, bb_period=20).generate_signals()
        assert "signal" in result.columns
        valid = {"buy", "sell", "hold"}
        assert set(result["signal"]).issubset(valid)

    def test_missing_bb_raises(self, ohlcv_50):
        from strategy.mean_reversion_strategy import MeanReversionStrategy

        with pytest.raises(ValueError, match="缺失布林带指标"):
            MeanReversionStrategy(ohlcv_50).generate_signals()


# ════════════════════════════════════════════
# Range Trading Strategy Tests
# ════════════════════════════════════════════

class TestRangeTradingStrategy:
    def test_basic_generation(self, ohlcv_50):
        from strategy.range_trading_strategy import RangeTradingStrategy

        data = _enrich_with_rsi(ohlcv_50, period=14)
        result = RangeTradingStrategy(
            data, rsi_period=14, rsi_low=30, rsi_high=70,
        ).generate_signals()
        assert "signal" in result.columns
        valid = {"buy", "sell", "hold"}
        assert set(result["signal"]).issubset(valid)

    def test_missing_rsi_raises(self, ohlcv_50):
        from strategy.range_trading_strategy import RangeTradingStrategy

        with pytest.raises(ValueError, match="缺失RSI指标"):
            RangeTradingStrategy(ohlcv_50).generate_signals()


# ════════════════════════════════════════════
# Grid Strategy Tests
# ════════════════════════════════════════════

class TestGridStrategy:
    def test_basic_generation(self, ohlcv_50):
        from strategy.grid_strategy import GridStrategy

        result = GridStrategy(ohlcv_50, grid_count=5, grid_step_pct=0.05).generate_signals()
        assert "signal" in result.columns
        assert "signal_size" in result.columns
        assert result["signal"].iloc[0] == "buy"

    def test_empty_dataframe(self):
        from strategy.grid_strategy import GridStrategy
        import pandas as pd

        df = pd.DataFrame(columns=["close"])
        result = GridStrategy(df, grid_count=3, grid_step_pct=0.05).generate_signals()
        assert len(result) == 0

    def test_single_row_dataframe(self):
        from strategy.grid_strategy import GridStrategy

        df = pd.DataFrame({"close": [100.0]})
        result = GridStrategy(df, grid_count=3, grid_step_pct=0.05).generate_signals()
        assert len(result) == 1
        assert result["signal"].iloc[0] == "buy"


# ════════════════════════════════════════════
# Strategy Registry Tests
# ════════════════════════════════════════════

class TestStrategyRegistry:
    def setup_method(self):
        from strategy_library.registry import StrategyRegistry
        self.registry = StrategyRegistry()

    def test_list_all_keys(self):
        keys = self.registry.list_implementation_keys()
        assert isinstance(keys, list)
        assert len(keys) == 8
        expected = {
            "bollinger_macd",
            "bollinger_reversion",
            "dual_confirm",
            "grid",
            "macd_cross",
            "rsi_range",
            "trend_cross",
            "volume_breakout",
        }
        assert set(keys) == expected

    def test_get_adapter_for_each_key(self):
        for key in self.registry.list_implementation_keys():
            adapter = self.registry.get_adapter(key)
            assert adapter is not None
            assert adapter.implementation_key == key
            assert callable(adapter.validate_params)
            assert callable(adapter.attach_indicators)
            assert callable(adapter.build_strategy)
            assert callable(adapter.get_overlay_columns)

    def test_get_unknown_key_raises(self):
        with pytest.raises(ValueError, match="未注册的策略实现: nonexistent"):
            self.registry.get_adapter("nonexistent")

    def test_validate_trend_params_ok(self):
        params = {"ma_short": "10", "ma_long": "30"}
        adapter = self.registry.get_adapter("trend_cross")
        adapter.validate_params(params)

    def test_validate_trend_params_bad(self):
        params = {"ma_short": "30", "ma_long": "10"}
        adapter = self.registry.get_adapter("trend_cross")
        with pytest.raises(ValueError, match="短均线周期小于长均线周期"):
            adapter.validate_params(params)

    def test_validate_grid_params_bad_count(self):
        adapter = self.registry.get_adapter("grid")
        with pytest.raises(ValueError, match="网格数量最小为 2"):
            adapter.validate_params({"grid_count": "1", "grid_step": "0.05"})

    def test_validate_grid_params_bad_step(self):
        adapter = self.registry.get_adapter("grid")
        with pytest.raises(ValueError, match="网格步长必须大于 0"):
            adapter.validate_params({"grid_count": "5", "grid_step": "0"})

    def test_validate_macd_params_bad_fast_ge_slow(self):
        adapter = self.registry.get_adapter("macd_cross")
        with pytest.raises(ValueError, match="快线周期必须小于慢线周期"):
            adapter.validate_params({"fast_period": "26", "slow_period": "12", "signal_period": "9"})

    def test_validate_macd_params_bad_signal(self):
        adapter = self.registry.get_adapter("macd_cross")
        with pytest.raises(ValueError, match="信号线周期最小为 2"):
            adapter.validate_params({"fast_period": "12", "slow_period": "26", "signal_period": "1"})

    def test_overlay_columns_return_list(self):
        for key in self.registry.list_implementation_keys():
            adapter = self.registry.get_adapter(key)
            cols = adapter.get_overlay_columns({"ma_short": "10", "ma_long": "30", "rsi_period": "14", "bb_period": "20", "bb_std": "2.0", "fast_period": "12", "slow_period": "26", "signal_period": "9", "logic_mode": "and", "grid_count": "5", "grid_step": "0.05", "volume_multiple": "2.0", "exit_ma_period": "20"})
            assert isinstance(cols, list)

    def test_dual_confirm_invalid_logic_mode(self):
        adapter = self.registry.get_adapter("dual_confirm")
        params = {
            "ma_short": "10", "ma_long": "30", "rsi_period": "14",
            "rsi_low": "35", "rsi_high": "70", "confirm_window": "5",
            "logic_mode": "xor",
        }
        with pytest.raises(ValueError, match="逻辑模式必须为 and 或 or"):
            adapter.validate_params(params)

    def test_bollinger_macd_invalid_logic_mode(self):
        adapter = self.registry.get_adapter("bollinger_macd")
        params = {
            "bb_period": "20", "bb_std": "2.0",
            "fast_period": "12", "slow_period": "26", "signal_period": "9",
            "logic_mode": "xor",
        }
        with pytest.raises(ValueError, match="逻辑模式必须为 and 或 or"):
            adapter.validate_params(params)


# ════════════════════════════════════════════
# Strategy Service Pure Logic Tests (use instance methods correctly)
# ════════════════════════════════════════════

class TestStrategyServicePureLogic:
    def _svc(self):
        from strategy_library.service import StrategyService
        return StrategyService()

    def test_utc_now_iso_format(self):
        result = self._svc()._utc_now_iso()
        assert result.endswith("Z")
        assert "T" in result

    def test_require_text_ok(self):
        assert self._svc()._require_text("hello", "field") == "hello"

    def test_require_text_empty_raises(self):
        with pytest.raises(ValueError, match="不能为空"):
            self._svc()._require_text("", "field")

    def test_require_text_none_raises(self):
        with pytest.raises(ValueError, match="不能为空"):
            self._svc()._require_text(None, "field")

    def test_normalize_strategy_name(self):
        assert self._svc()._normalize_strategy_name(" Hello World ") == "helloworld"
        assert self._svc()._normalize_strategy_name("趋势（双均线）") == "趋势(双均线)"
        assert self._svc()._normalize_strategy_name("") == ""

    def test_coerce_integer_ok(self):
        from strategy_library.models import StrategyParamDefinition
        item = StrategyParamDefinition(key="x", label="X", type="integer")
        svc = self._svc()
        assert svc._coerce_param_value(item, "42") == 42
        assert svc._coerce_param_value(item, 42.0) == 42

    def test_coerce_integer_rejects_bool(self):
        from strategy_library.models import StrategyParamDefinition
        item = StrategyParamDefinition(key="x", label="X", type="integer")
        with pytest.raises(ValueError):
            self._svc()._coerce_param_value(item, True)

    def test_coerce_number_ok(self):
        from strategy_library.models import StrategyParamDefinition
        item = StrategyParamDefinition(key="x", label="X", type="number")
        assert self._svc()._coerce_param_value(item, "3.14") == 3.14

    def test_coerce_boolean_ok(self):
        from strategy_library.models import StrategyParamDefinition
        item = StrategyParamDefinition(key="x", label="X", type="boolean")
        svc = self._svc()
        assert svc._coerce_param_value(item, "true") is True
        assert svc._coerce_param_value(item, "false") is False
        assert svc._coerce_param_value(item, "yes") is True
        assert svc._coerce_param_value(item, "0") is False

    def test_resolve_version_new(self):
        assert self._svc()._resolve_version(1, None) == 1
        assert self._svc()._resolve_version(3, None) == 3

    def test_resolve_version_bumps_on_update(self):
        from strategy_library.models import StrategyDefinition
        existing = StrategyDefinition(
            id="x", key="k", name="n", implementation_key="tk",
            created_at="Z", updated_at="Z", version=2,
        )
        svc = self._svc()
        assert svc._resolve_version(2, existing) == max(2, existing.version + 1)
        assert svc._resolve_version(1, existing) == max(1, existing.version + 1)

    def test_merge_with_defaults(self):
        from strategy_library.models import StrategyDefinition, StrategyParamDefinition
        strat = StrategyDefinition(
            id="s1", key="k1", name="S1", implementation_key="trend_cross",
            created_at="Z", updated_at="Z",
            param_schema=[
                StrategyParamDefinition(key="ma_short", label="Short", type="integer", default=10, min=2, max=250),
                StrategyParamDefinition(key="ma_long", label="Long", type="integer", default=60, min=3, max=500),
            ],
            default_params={"ma_short": 10, "ma_long": 60},
        )
        svc = self._svc()
        merged = svc.merge_with_defaults(strat, {"ma_long": "120"})
        assert merged["ma_short"] == 10
        assert merged["ma_long"] == 120


# ════════════════════════════════════════════
# Strategy Library Model Tests
# ════════════════════════════════════════════

class TestStrategyLibraryModels:
    def test_param_definition_defaults(self):
        from strategy_library.models import StrategyParamDefinition
        p = StrategyParamDefinition(key="test", label="Test")
        assert p.type == "number"
        assert p.required is True
        assert p.options == []

    def test_strategy_definition_defaults(self):
        from strategy_library.models import StrategyDefinition
        s = StrategyDefinition(
            id="id1", key="k1", name="N1", implementation_key="tc",
            created_at="Z", updated_at="Z",
        )
        assert s.status == "draft"
        assert s.version == 1
        assert s.category == "通用"
