from dataclasses import dataclass
from typing import Any, Callable, Dict, List

import pandas as pd

from indicators.technical_indicators import TechnicalIndicators
from strategy.bollinger_macd_strategy import BollingerMACDStrategy
from strategy.combo_grid_strategy import ComboGridStrategy
from strategy.dual_confirm_strategy import DualConfirmStrategy
from strategy.grid_strategy import GridStrategy
from strategy.macd_strategy import MACDStrategy
from strategy.mean_reversion_strategy import MeanReversionStrategy
from strategy.range_trading_strategy import RangeTradingStrategy
from strategy.trend_strategy import TrendStrategy
from strategy.volume_breakout_strategy import VolumeBreakoutStrategy


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
            "macd_cross": StrategyExecutionAdapter(
                implementation_key="macd_cross",
                validate_params=_validate_macd_params,
                attach_indicators=_attach_macd_indicators,
                build_strategy=_build_macd_strategy,
                get_overlay_columns=_macd_overlay_columns,
            ),
            "volume_breakout": StrategyExecutionAdapter(
                implementation_key="volume_breakout",
                validate_params=_validate_volume_breakout_params,
                attach_indicators=_attach_volume_breakout_indicators,
                build_strategy=_build_volume_breakout_strategy,
                get_overlay_columns=_volume_breakout_overlay_columns,
            ),
            "dual_confirm": StrategyExecutionAdapter(
                implementation_key="dual_confirm",
                validate_params=_validate_dual_confirm_params,
                attach_indicators=_attach_dual_confirm_indicators,
                build_strategy=_build_dual_confirm_strategy,
                get_overlay_columns=_dual_confirm_overlay_columns,
            ),
            "bollinger_macd": StrategyExecutionAdapter(
                implementation_key="bollinger_macd",
                validate_params=_validate_bollinger_macd_params,
                attach_indicators=_attach_bollinger_macd_indicators,
                build_strategy=_build_bollinger_macd_strategy,
                get_overlay_columns=_bollinger_macd_overlay_columns,
            ),
            "combo_grid": StrategyExecutionAdapter(
                implementation_key="combo_grid",
                validate_params=_validate_combo_grid_params,
                attach_indicators=_attach_combo_grid_indicators,
                build_strategy=_build_combo_grid_strategy,
                get_overlay_columns=_combo_grid_overlay_columns,
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


# ── MACD 趋势策略 ──


def _validate_macd_params(params: Dict[str, Any]) -> None:
    if int(params["fast_period"]) >= int(params["slow_period"]):
        raise ValueError("MACD 快线周期必须小于慢线周期")
    if int(params["signal_period"]) < 2:
        raise ValueError("MACD 信号线周期最小为 2")


def _attach_macd_indicators(data: pd.DataFrame, params: Dict[str, Any]) -> pd.DataFrame:
    indicator_calc = TechnicalIndicators(data)
    enriched = indicator_calc.data.copy()
    dif, dea, histogram = indicator_calc.calculate_macd(
        fast_period=int(params["fast_period"]),
        slow_period=int(params["slow_period"]),
        signal_period=int(params["signal_period"]),
    )
    enriched["MACD_DIF"] = dif
    enriched["MACD_DEA"] = dea
    enriched["MACD_HIST"] = histogram
    return enriched


def _build_macd_strategy(data: pd.DataFrame, params: Dict[str, Any]) -> MACDStrategy:
    return MACDStrategy(
        data,
        fast_period=int(params["fast_period"]),
        slow_period=int(params["slow_period"]),
        signal_period=int(params["signal_period"]),
    )


def _macd_overlay_columns(params: Dict[str, Any]) -> List[str]:
    return ["MACD_DIF", "MACD_DEA", "MACD_HIST"]


# ── 放量突破策略 ──


def _validate_volume_breakout_params(params: Dict[str, Any]) -> None:
    if int(params["lookback"]) < 5:
        raise ValueError("回看周期最小为 5")
    if float(params["volume_multiple"]) < 1.0:
        raise ValueError("放量倍数必须 >= 1.0")
    if int(params["exit_ma_period"]) < 5:
        raise ValueError("离场均线周期最小为 5")


def _attach_volume_breakout_indicators(data: pd.DataFrame, params: Dict[str, Any]) -> pd.DataFrame:
    indicator_calc = TechnicalIndicators(data)
    enriched = indicator_calc.data.copy()
    lookback = int(params["lookback"])
    exit_ma = int(params["exit_ma_period"])
    enriched[f"VOL_MA{lookback}"] = enriched["volume"].rolling(window=lookback, min_periods=lookback).mean()
    enriched[f"HIGH_{lookback}"] = enriched["high"].rolling(window=lookback, min_periods=lookback).max()
    enriched[f"MA{exit_ma}"] = indicator_calc.calculate_ma(exit_ma)
    return enriched


def _build_volume_breakout_strategy(data: pd.DataFrame, params: Dict[str, Any]) -> VolumeBreakoutStrategy:
    return VolumeBreakoutStrategy(
        data,
        lookback=int(params["lookback"]),
        volume_multiple=float(params["volume_multiple"]),
        exit_ma_period=int(params["exit_ma_period"]),
    )


def _volume_breakout_overlay_columns(params: Dict[str, Any]) -> List[str]:
    exit_ma = int(params["exit_ma_period"])
    return [f"MA{exit_ma}"]


# ── 双重确认策略（趋势+动量组合） ──

ALLOWED_LOGIC_MODES = {"and", "or"}


def _validate_dual_confirm_params(params: Dict[str, Any]) -> None:
    if int(params["ma_short"]) >= int(params["ma_long"]):
        raise ValueError("短均线周期必须小于长均线周期")
    if float(params["rsi_low"]) >= float(params["rsi_high"]):
        raise ValueError("RSI 低阈值必须小于高阈值")
    if int(params["confirm_window"]) < 1:
        raise ValueError("确认窗口最小为 1 天")
    mode = str(params.get("logic_mode", "and")).strip().lower()
    if mode not in ALLOWED_LOGIC_MODES:
        raise ValueError(f"逻辑模式必须为 and 或 or，当前值: {mode}")


def _attach_dual_confirm_indicators(data: pd.DataFrame, params: Dict[str, Any]) -> pd.DataFrame:
    indicator_calc = TechnicalIndicators(data)
    enriched = indicator_calc.data.copy()
    short_p = int(params["ma_short"])
    long_p = int(params["ma_long"])
    rsi_p = int(params["rsi_period"])
    enriched[f"MA{short_p}"] = indicator_calc.calculate_ma(short_p)
    enriched[f"MA{long_p}"] = indicator_calc.calculate_ma(long_p)
    enriched[f"RSI_{rsi_p}"] = indicator_calc.calculate_rsi(period=rsi_p)
    return enriched


def _build_dual_confirm_strategy(data: pd.DataFrame, params: Dict[str, Any]) -> DualConfirmStrategy:
    return DualConfirmStrategy(
        data,
        ma_short=int(params["ma_short"]),
        ma_long=int(params["ma_long"]),
        rsi_period=int(params["rsi_period"]),
        rsi_low=float(params["rsi_low"]),
        rsi_high=float(params["rsi_high"]),
        confirm_window=int(params["confirm_window"]),
        logic_mode=str(params.get("logic_mode", "and")),
    )


def _dual_confirm_overlay_columns(params: Dict[str, Any]) -> List[str]:
    return [
        f"MA{int(params['ma_short'])}",
        f"MA{int(params['ma_long'])}",
        f"RSI_{int(params['rsi_period'])}",
    ]


# ── 布林带 + MACD 组合策略 ──


def _validate_bollinger_macd_params(params: Dict[str, Any]) -> None:
    if int(params["bb_period"]) < 5:
        raise ValueError("布林带周期最小为 5")
    if float(params["bb_std"]) <= 0:
        raise ValueError("布林带标准差倍数必须大于 0")
    if int(params["fast_period"]) >= int(params["slow_period"]):
        raise ValueError("MACD 快线周期必须小于慢线周期")
    if int(params["signal_period"]) < 2:
        raise ValueError("MACD 信号线周期最小为 2")
    mode = str(params.get("logic_mode", "and")).strip().lower()
    if mode not in ALLOWED_LOGIC_MODES:
        raise ValueError(f"逻辑模式必须为 and 或 or，当前值: {mode}")


def _attach_bollinger_macd_indicators(data: pd.DataFrame, params: Dict[str, Any]) -> pd.DataFrame:
    indicator_calc = TechnicalIndicators(data)
    enriched = indicator_calc.data.copy()
    upper, mid, lower = indicator_calc.calculate_bollinger_bands(
        period=int(params["bb_period"]), std_dev=float(params["bb_std"])
    )
    enriched["BB_upper"] = upper
    enriched["BB_mid"] = mid
    enriched["BB_lower"] = lower
    dif, dea, histogram = indicator_calc.calculate_macd(
        fast_period=int(params["fast_period"]),
        slow_period=int(params["slow_period"]),
        signal_period=int(params["signal_period"]),
    )
    enriched["MACD_DIF"] = dif
    enriched["MACD_DEA"] = dea
    enriched["MACD_HIST"] = histogram
    return enriched


def _build_bollinger_macd_strategy(data: pd.DataFrame, params: Dict[str, Any]) -> BollingerMACDStrategy:
    return BollingerMACDStrategy(
        data,
        bb_period=int(params["bb_period"]),
        bb_std=float(params["bb_std"]),
        fast_period=int(params["fast_period"]),
        slow_period=int(params["slow_period"]),
        signal_period=int(params["signal_period"]),
        logic_mode=str(params.get("logic_mode", "and")),
    )


def _bollinger_macd_overlay_columns(params: Dict[str, Any]) -> List[str]:
    return ["BB_upper", "BB_mid", "BB_lower", "MACD_DIF", "MACD_DEA"]


# ── 组合策略：网格 + RSI/布林择时 + 突破熔断 ──

ALLOWED_BOX_METHODS = {"minmax", "boll", "percentile"}


def _validate_combo_grid_params(params: Dict[str, Any]) -> None:
    grid_ratio = float(params["grid_capital_ratio"])
    timing_ratio = float(params["timing_capital_ratio"])
    break_ratio = float(params["break_capital_ratio"])
    if abs(grid_ratio + timing_ratio + break_ratio - 1.0) > 1e-6:
        raise ValueError("三部分资金比例之和必须等于 1（网格 + 择时 + 突破）")
    if int(params["grid_num"]) < 2:
        raise ValueError("网格档数最小为 2")
    if float(params["rsi_oversold"]) >= float(params["rsi_overbought"]):
        raise ValueError("RSI 超卖阈值必须小于超买阈值")
    box_method = str(params.get("box_method", "minmax")).strip().lower()
    if box_method not in ALLOWED_BOX_METHODS:
        raise ValueError(f"箱体方式必须为 minmax / boll / percentile，当前值: {box_method}")
    if int(params["box_lookback"]) < 5:
        raise ValueError("箱体窗口最小为 5")
    if float(params["boll_std"]) <= 0:
        raise ValueError("布林带标准差倍数必须大于 0")


def _attach_combo_grid_indicators(data: pd.DataFrame, params: Dict[str, Any]) -> pd.DataFrame:
    """移植自 combo_strategy.compute_indicators：动态箱体 + 布林 + RSI + 均量 + 关键阈值。

    所有派生列加 combo_ 前缀，避免与其它策略/指标列冲突。
    """
    enriched = data.copy()

    box_method = str(params.get("box_method", "minmax")).strip().lower()
    box_lookback = int(params["box_lookback"])
    box_pctile = float(params.get("box_pctile", 0.10))
    boll_window = int(params.get("boll_window", 20))
    boll_std = float(params["boll_std"])
    grid_num = int(params["grid_num"])
    rsi_window = int(params.get("rsi_window", 14))
    vol_window = int(params.get("vol_window", 20))
    grid_stop_pct = float(params["grid_stop_pct"])
    break_buffer = float(params.get("break_buffer", 0.0))
    break_down_buffer = float(params.get("break_down_buffer", 0.0))

    # 动态箱体上下沿
    if box_method == "minmax":
        enriched["combo_box_up"] = enriched["high"].rolling(box_lookback).max()
        enriched["combo_box_low"] = enriched["low"].rolling(box_lookback).min()
    elif box_method == "percentile":
        enriched["combo_box_up"] = enriched["close"].rolling(box_lookback).quantile(1 - box_pctile)
        enriched["combo_box_low"] = enriched["close"].rolling(box_lookback).quantile(box_pctile)
    else:  # boll
        ma = enriched["close"].rolling(boll_window).mean()
        sd = enriched["close"].rolling(boll_window).std()
        enriched["combo_box_up"] = ma + boll_std * sd
        enriched["combo_box_low"] = ma - boll_std * sd

    enriched["combo_grid_step"] = (enriched["combo_box_up"] - enriched["combo_box_low"]) / grid_num

    # 布林带（择时用，独立于箱体）
    ma = enriched["close"].rolling(boll_window).mean()
    sd = enriched["close"].rolling(boll_window).std()
    enriched["combo_boll_up"] = ma + boll_std * sd
    enriched["combo_boll_low"] = ma - boll_std * sd

    # RSI（与用户原脚本一致的算法）
    delta = enriched["close"].diff()
    gain = delta.clip(lower=0).rolling(rsi_window).mean()
    loss = (-delta.clip(upper=0)).rolling(rsi_window).mean()
    rs = gain / loss.replace(0, float("nan"))
    enriched["combo_rsi"] = 100 - 100 / (1 + rs)

    # 均量
    enriched["combo_vol_ma"] = enriched["volume"].rolling(vol_window).mean()

    # 关键阈值（相对价格，每根 K 线动态）
    enriched["combo_stop_line"] = enriched["combo_box_low"] * (1 - grid_stop_pct)
    enriched["combo_break_up_line"] = enriched["combo_box_up"] * (1 + break_buffer)
    enriched["combo_break_down_line"] = enriched["combo_box_low"] * (1 - break_down_buffer)
    return enriched


def _build_combo_grid_strategy(data: pd.DataFrame, params: Dict[str, Any]) -> ComboGridStrategy:
    return ComboGridStrategy(
        data,
        grid_num=int(params["grid_num"]),
        grid_capital_ratio=float(params["grid_capital_ratio"]),
        grid_stop_pct=float(params["grid_stop_pct"]),
        rsi_oversold=float(params["rsi_oversold"]),
        rsi_overbought=float(params["rsi_overbought"]),
        timing_capital_ratio=float(params["timing_capital_ratio"]),
        break_buffer=float(params.get("break_buffer", 0.0)),
        vol_multiple=float(params["vol_multiple"]),
        trail_stop_pct=float(params["trail_stop_pct"]),
        break_capital_ratio=float(params["break_capital_ratio"]),
    )


def _combo_grid_overlay_columns(params: Dict[str, Any]) -> List[str]:
    return ["combo_box_up", "combo_box_low", "combo_boll_up", "combo_boll_low"]
