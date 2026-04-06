"""
布林带 + MACD 组合策略模块
"""
import pandas as pd
import numpy as np


class BollingerMACDStrategy:
    """布林带 + MACD 组合策略（底/顶部共振）

    买入逻辑:
      - AND 模式: 价格触及布林带下轨 AND MACD 柱状图由负转正
      - OR 模式: 任一触发即买入

    卖出逻辑:
      - AND 模式: 价格触及布林带上轨 AND MACD 柱状图由正转负
      - OR 模式: 任一触发即卖出
    """

    def __init__(
        self,
        data: pd.DataFrame,
        bb_period: int = 20,
        bb_std: float = 2.0,
        fast_period: int = 12,
        slow_period: int = 26,
        signal_period: int = 9,
        logic_mode: str = "and",
    ):
        self.data = data.copy()
        self.bb_period = bb_period
        self.bb_std = bb_std
        self.fast_period = fast_period
        self.slow_period = slow_period
        self.signal_period = signal_period
        self.logic_mode = logic_mode.strip().lower()

    def generate_signals(self) -> pd.DataFrame:
        mode_label = self.logic_mode.upper()
        print(
            f"📡 生成布林带+MACD 组合信号 ({mode_label} 模式, "
            f"BB{self.bb_period}/{self.bb_std} + MACD {self.fast_period}/{self.slow_period}/{self.signal_period})..."
        )
        self.data["signal"] = "hold"
        self.data["signal_size"] = 1.0

        required = ["BB_upper", "BB_lower", "MACD_HIST"]
        for col in required:
            if col not in self.data.columns:
                raise ValueError(f"缺失指标 ({col})")

        n = len(self.data)
        bb_buy = [False] * n
        bb_sell = [False] * n
        macd_buy = [False] * n
        macd_sell = [False] * n

        for i in range(1, n):
            close = self.data["close"].iloc[i]
            close_prev = self.data["close"].iloc[i - 1]
            bb_lower = self.data["BB_lower"].iloc[i]
            bb_lower_prev = self.data["BB_lower"].iloc[i - 1]
            bb_upper = self.data["BB_upper"].iloc[i]
            bb_upper_prev = self.data["BB_upper"].iloc[i - 1]
            hist = self.data["MACD_HIST"].iloc[i]
            hist_prev = self.data["MACD_HIST"].iloc[i - 1]

            if any(pd.isna(v) for v in [close, bb_lower, bb_upper, hist, hist_prev]):
                continue

            # Bollinger sub-signals
            if close_prev >= bb_lower_prev and close < bb_lower:
                bb_buy[i] = True
            if close_prev <= bb_upper_prev and close > bb_upper:
                bb_sell[i] = True

            # MACD histogram sub-signals
            if hist_prev <= 0 and hist > 0:
                macd_buy[i] = True
            if hist_prev >= 0 and hist < 0:
                macd_sell[i] = True

        if self.logic_mode == "or":
            self._apply_or(bb_buy, bb_sell, macd_buy, macd_sell)
        else:
            self._apply_and(bb_buy, bb_sell, macd_buy, macd_sell)

        return self.data

    def _apply_and(self, bb_buy, bb_sell, macd_buy, macd_sell):
        """AND: 两个子信号在同一天或 3 天窗口内先后触发"""
        n = len(self.data)
        w = 3  # 短窗口：布林带触及和 MACD 翻转往往不会同一天

        def has_recent(arr, i):
            start = max(0, i - w)
            return any(arr[start : i + 1])

        for i in range(1, n):
            if bb_buy[i] and has_recent(macd_buy, i):
                self.data.loc[self.data.index[i], "signal"] = "buy"
            elif macd_buy[i] and has_recent(bb_buy, i):
                self.data.loc[self.data.index[i], "signal"] = "buy"
            elif bb_sell[i] and has_recent(macd_sell, i):
                self.data.loc[self.data.index[i], "signal"] = "sell"
            elif macd_sell[i] and has_recent(bb_sell, i):
                self.data.loc[self.data.index[i], "signal"] = "sell"

    def _apply_or(self, bb_buy, bb_sell, macd_buy, macd_sell):
        """OR: 任一子信号触发即生效"""
        for i in range(1, len(self.data)):
            if bb_buy[i] or macd_buy[i]:
                self.data.loc[self.data.index[i], "signal"] = "buy"
            elif bb_sell[i] or macd_sell[i]:
                self.data.loc[self.data.index[i], "signal"] = "sell"
