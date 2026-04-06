"""
MACD 趋势策略模块
"""
import pandas as pd


class MACDStrategy:
    """MACD 趋势策略（DIF/DEA 交叉）"""

    def __init__(
        self,
        data: pd.DataFrame,
        fast_period: int = 12,
        slow_period: int = 26,
        signal_period: int = 9,
    ):
        self.data = data.copy()
        self.fast_period = fast_period
        self.slow_period = slow_period
        self.signal_period = signal_period

    def generate_signals(self) -> pd.DataFrame:
        print(
            f"📡 生成 MACD 交叉信号 (快线:{self.fast_period}, "
            f"慢线:{self.slow_period}, 信号线:{self.signal_period})..."
        )
        self.data["signal"] = "hold"
        self.data["signal_size"] = 1.0

        dif_col = "MACD_DIF"
        dea_col = "MACD_DEA"

        if dif_col not in self.data.columns or dea_col not in self.data.columns:
            raise ValueError(f"缺失 MACD 指标 ({dif_col}, {dea_col})")

        for i in range(1, len(self.data)):
            if pd.isna(self.data[dif_col].iloc[i]) or pd.isna(self.data[dea_col].iloc[i]):
                continue

            dif_today = self.data[dif_col].iloc[i]
            dea_today = self.data[dea_col].iloc[i]
            dif_yesterday = self.data[dif_col].iloc[i - 1]
            dea_yesterday = self.data[dea_col].iloc[i - 1]

            # 金叉: DIF 从下方上穿 DEA
            if dif_yesterday <= dea_yesterday and dif_today > dea_today:
                self.data.loc[self.data.index[i], "signal"] = "buy"

            # 死叉: DIF 从上方下穿 DEA
            elif dif_yesterday >= dea_yesterday and dif_today < dea_today:
                self.data.loc[self.data.index[i], "signal"] = "sell"

        return self.data
