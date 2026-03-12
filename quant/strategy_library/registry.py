from dataclasses import dataclass
from typing import Any, Callable, Dict, List

import pandas as pd

from indicators.technical_indicators import TechnicalIndicators
from strategy.grid_strategy import GridStrategy
from strategy.mean_reversion_strategy import MeanReversionStrategy
from strategy.range_trading_strategy import RangeTradingStrategy
from strategy.trend_strategy import TrendStrategy


@dataclass
class StrategyExecutionAdapter:
    implementation_key: str
    validate_params: Callable[[Dict[str, Any]], None]
    attach_indicators: Callable[[pd.DataFrame, Dict[str, Any]], pd.DataFrame]
    build_strategy: Callable[[pd.DataFrame, Dict[str, Any]], Any]
    get_overlay_columns: Callable[[Dict[str, Any]], List[str]]


class StrategyRegistry:
    def __init__(self):
        self._adapters: Dict[str, StrategyExecutionAdapter] = {
            "trend_cross": StrategyExecutionAdapter(
                implementation_key="trend_cross",
                validate_params=_validate_trend_params,
                attach_indicators=_attach_trend_indicators,
                build_strategy=_build_trend_strategy,
                get_overlay_columns=_trend_overlay_columns,
            ),
            "grid": StrategyExecutionAdapter(
                implementation_key="grid",
                validate_params=_validate_grid_params,
                attach_indicators=_attach_grid_indicators,
                build_strategy=_build_grid_strategy,
                get_overlay_columns=_grid_overlay_columns,
            ),
            "bollinger_reversion": StrategyExecutionAdapter(
                implementation_key="bollinger_reversion",
                validate_params=_validate_bollinger_params,
                attach_indicators=_attach_bollinger_indicators,
                build_strategy=_build_bollinger_strategy,
                get_overlay_columns=_bollinger_overlay_columns,
            ),
            "rsi_range": StrategyExecutionAdapter(
                implementation_key="rsi_range",
                validate_params=_validate_rsi_params,
                attach_indicators=_attach_rsi_indicators,
                build_strategy=_build_rsi_strategy,
                get_overlay_columns=_rsi_overlay_columns,
            ),
        }

    def list_implementation_keys(self) -> List[str]:
        return sorted(self._adapters.keys())

    def get_adapter(self, implementation_key: str) -> StrategyExecutionAdapter:
        adapter = self._adapters.get(implementation_key)
        if adapter is None:
            raise ValueError(f"未注册的策略实现: {implementation_key}")
        return adapter


def _validate_trend_params(params: Dict[str, Any]) -> None:
    if int(params["ma_short"]) >= int(params["ma_long"]):
        raise ValueError("双均线策略要求短均线周期小于长均线周期")


def _attach_trend_indicators(data: pd.DataFrame, params: Dict[str, Any]) -> pd.DataFrame:
    indicator_calc = TechnicalIndicators(data)
    enriched = indicator_calc.data.copy()
    short_period = int(params["ma_short"])
    long_period = int(params["ma_long"])
    enriched[f"MA{short_period}"] = indicator_calc.calculate_ma(short_period)
    enriched[f"MA{long_period}"] = indicator_calc.calculate_ma(long_period)
    return enriched


def _build_trend_strategy(data: pd.DataFrame, params: Dict[str, Any]) -> TrendStrategy:
    return TrendStrategy(data, ma_short=int(params["ma_short"]), ma_long=int(params["ma_long"]))


def _trend_overlay_columns(params: Dict[str, Any]) -> List[str]:
    return [f"MA{int(params['ma_short'])}", f"MA{int(params['ma_long'])}"]


def _validate_grid_params(params: Dict[str, Any]) -> None:
    if int(params["grid_count"]) < 2:
        raise ValueError("网格数量最小为 2")
    if float(params["grid_step"]) <= 0:
        raise ValueError("网格步长必须大于 0")


def _attach_grid_indicators(data: pd.DataFrame, params: Dict[str, Any]) -> pd.DataFrame:
    return data.copy()


def _build_grid_strategy(data: pd.DataFrame, params: Dict[str, Any]) -> GridStrategy:
    return GridStrategy(data, grid_count=int(params["grid_count"]), grid_step_pct=float(params["grid_step"]))


def _grid_overlay_columns(params: Dict[str, Any]) -> List[str]:
    return []


def _validate_bollinger_params(params: Dict[str, Any]) -> None:
    if int(params["bb_period"]) < 5:
        raise ValueError("布林带周期最小为 5")
    if float(params["bb_std"]) <= 0:
        raise ValueError("布林带标准差倍数必须大于 0")


def _attach_bollinger_indicators(data: pd.DataFrame, params: Dict[str, Any]) -> pd.DataFrame:
    indicator_calc = TechnicalIndicators(data)
    enriched = indicator_calc.data.copy()
    upper_band, mid_band, lower_band = indicator_calc.calculate_bollinger_bands(
        period=int(params["bb_period"]), std_dev=float(params["bb_std"])
    )
    enriched["BB_upper"] = upper_band
    enriched["BB_mid"] = mid_band
    enriched["BB_lower"] = lower_band
    return enriched


def _build_bollinger_strategy(data: pd.DataFrame, params: Dict[str, Any]) -> MeanReversionStrategy:
    return MeanReversionStrategy(data, bb_period=int(params["bb_period"]))


def _bollinger_overlay_columns(params: Dict[str, Any]) -> List[str]:
    return ["BB_upper", "BB_mid", "BB_lower"]


def _validate_rsi_params(params: Dict[str, Any]) -> None:
    if float(params["rsi_low"]) >= float(params["rsi_high"]):
        raise ValueError("RSI 低阈值必须小于高阈值")


def _attach_rsi_indicators(data: pd.DataFrame, params: Dict[str, Any]) -> pd.DataFrame:
    indicator_calc = TechnicalIndicators(data)
    enriched = indicator_calc.data.copy()
    period = int(params["rsi_period"])
    enriched[f"RSI_{period}"] = indicator_calc.calculate_rsi(period=period)
    return enriched


def _build_rsi_strategy(data: pd.DataFrame, params: Dict[str, Any]) -> RangeTradingStrategy:
    return RangeTradingStrategy(
        data,
        rsi_period=int(params["rsi_period"]),
        rsi_low=float(params["rsi_low"]),
        rsi_high=float(params["rsi_high"]),
    )


def _rsi_overlay_columns(params: Dict[str, Any]) -> List[str]:
    return [f"RSI_{int(params['rsi_period'])}"]
