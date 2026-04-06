"""
放量突破策略模块
"""
import pandas as pd
import numpy as np


class VolumeBreakoutStrategy:
    """放量突破策略（量价齐升）"""

    def __init__(
        self,
        data: pd.DataFrame,
        lookback: int = 20,
        volume_multiple: float = 2.0,
        exit_ma_period: int = 20,
    ):
        self.data = data.copy()
        self.lookback = lookback
        self.volume_multiple = volume_multiple
        self.exit_ma_period = exit_ma_period

    def generate_signals(self) -> pd.DataFrame:
        print(
            f"📡 生成放量突破信号 (回看:{self.lookback}天, "
            f"放量倍数:{self.volume_multiple}x, 离场均线:MA{self.exit_ma_period})..."
        )
        self.data["signal"] = "hold"
        self.data["signal_size"] = 1.0

        vol_ma_col = f"VOL_MA{self.lookback}"
        high_col = f"HIGH_{self.lookback}"
        exit_ma_col = f"MA{self.exit_ma_period}"

        if vol_ma_col not in self.data.columns:
            raise ValueError(f"缺失均量指标 ({vol_ma_col})")
        if high_col not in self.data.columns:
            raise ValueError(f"缺失区间最高价指标 ({high_col})")
        if exit_ma_col not in self.data.columns:
            raise ValueError(f"缺失离场均线指标 ({exit_ma_col})")

        in_position = False

        for i in range(1, len(self.data)):
            vol_ma = self.data[vol_ma_col].iloc[i]
            high_n = self.data[high_col].iloc[i - 1]  # 前 N 日最高价（不含当日）
            exit_ma = self.data[exit_ma_col].iloc[i]
            close = self.data["close"].iloc[i]
            volume = self.data["volume"].iloc[i]

            if pd.isna(vol_ma) or pd.isna(high_n) or pd.isna(exit_ma):
                continue

            if not in_position:
                # 买入条件: 放量（当日量 > 均量 × 倍数）且收盘价创 N 日新高
                if volume > vol_ma * self.volume_multiple and close > high_n:
                    self.data.loc[self.data.index[i], "signal"] = "buy"
                    in_position = True
            else:
                # 卖出条件: 收盘价跌破离场均线
                if close < exit_ma:
                    self.data.loc[self.data.index[i], "signal"] = "sell"
                    in_position = False

        return self.data
