"""
双重确认策略模块（趋势 + 动量组合）
"""
import pandas as pd
import numpy as np


class DualConfirmStrategy:
    """双重确认策略：均线交叉（趋势）+ RSI（动量）组合

    买入逻辑:
      - AND 模式: 两个信号在 confirm_window 天内先后触发才买入
      - OR 模式: 任一信号触发即买入

    卖出逻辑:
      - AND 模式: 两个信号同时满足才卖出
      - OR 模式: 任一信号触发即卖出（更保守）
    """

    def __init__(
        self,
        data: pd.DataFrame,
        ma_short: int = 10,
        ma_long: int = 30,
        rsi_period: int = 14,
        rsi_low: float = 35.0,
        rsi_high: float = 70.0,
        confirm_window: int = 5,
        logic_mode: str = "and",
    ):
        self.data = data.copy()
        self.ma_short = ma_short
        self.ma_long = ma_long
        self.rsi_period = rsi_period
        self.rsi_low = rsi_low
        self.rsi_high = rsi_high
        self.confirm_window = confirm_window
        self.logic_mode = logic_mode.strip().lower()

    def generate_signals(self) -> pd.DataFrame:
        mode_label = self.logic_mode.upper()
        print(
            f"📡 生成双重确认信号 ({mode_label} 模式, "
            f"MA{self.ma_short}/{self.ma_long} + RSI{self.rsi_period}, "
            f"窗口:{self.confirm_window}天)..."
        )
        self.data["signal"] = "hold"
        self.data["signal_size"] = 1.0

        ma_short_col = f"MA{self.ma_short}"
        ma_long_col = f"MA{self.ma_long}"
        rsi_col = f"RSI_{self.rsi_period}"

        for col in [ma_short_col, ma_long_col, rsi_col]:
            if col not in self.data.columns:
                raise ValueError(f"缺失指标 ({col})")

        # Pre-compute raw sub-signals
        n = len(self.data)
        trend_buy = [False] * n
        trend_sell = [False] * n
        rsi_buy = [False] * n
        rsi_sell = [False] * n

        for i in range(1, n):
            ma_s = self.data[ma_short_col].iloc[i]
            ma_l = self.data[ma_long_col].iloc[i]
            ma_s_prev = self.data[ma_short_col].iloc[i - 1]
            ma_l_prev = self.data[ma_long_col].iloc[i - 1]
            rsi = self.data[rsi_col].iloc[i]
            rsi_prev = self.data[rsi_col].iloc[i - 1]

            if pd.isna(ma_s) or pd.isna(ma_l) or pd.isna(rsi) or pd.isna(rsi_prev):
                continue

            # Trend sub-signals
            if ma_s_prev <= ma_l_prev and ma_s > ma_l:
                trend_buy[i] = True
            if ma_s_prev >= ma_l_prev and ma_s < ma_l:
                trend_sell[i] = True

            # RSI sub-signals
            if rsi_prev <= self.rsi_low and rsi > self.rsi_low:
                rsi_buy[i] = True
            if rsi_prev >= self.rsi_high and rsi < self.rsi_high:
                rsi_sell[i] = True

        # Combine with logic mode
        if self.logic_mode == "or":
            self._apply_or_logic(trend_buy, trend_sell, rsi_buy, rsi_sell)
        else:
            self._apply_and_logic(trend_buy, trend_sell, rsi_buy, rsi_sell)

        return self.data

    def _apply_and_logic(self, trend_buy, trend_sell, rsi_buy, rsi_sell):
        """AND 模式：两个子信号在窗口内先后触发才生效"""
        n = len(self.data)
        w = self.confirm_window

        # Helper: check if any True in arr[max(0,i-w):i+1]
        def has_recent(arr, i):
            start = max(0, i - w)
            return any(arr[start : i + 1])

        for i in range(1, n):
            # Buy: both trend_buy and rsi_buy fire within window
            if trend_buy[i] and has_recent(rsi_buy, i):
                self.data.loc[self.data.index[i], "signal"] = "buy"
            elif rsi_buy[i] and has_recent(trend_buy, i):
                self.data.loc[self.data.index[i], "signal"] = "buy"
            # Sell: both trend_sell and rsi_sell fire within window
            elif trend_sell[i] and has_recent(rsi_sell, i):
                self.data.loc[self.data.index[i], "signal"] = "sell"
            elif rsi_sell[i] and has_recent(trend_sell, i):
                self.data.loc[self.data.index[i], "signal"] = "sell"

    def _apply_or_logic(self, trend_buy, trend_sell, rsi_buy, rsi_sell):
        """OR 模式：任一子信号触发即生效"""
        for i in range(1, len(self.data)):
            if trend_buy[i] or rsi_buy[i]:
                self.data.loc[self.data.index[i], "signal"] = "buy"
            elif trend_sell[i] or rsi_sell[i]:
                self.data.loc[self.data.index[i], "signal"] = "sell"
