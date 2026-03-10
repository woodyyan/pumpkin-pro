"""
策略模块 - 负责生成交易信号
"""

import pandas as pd
from typing import Dict, List, Optional
from enum import Enum

from config import MA_SHORT, MA_LONG


class Signal(Enum):
    """交易信号枚举"""

    BUY = "buy"
    SELL = "sell"
    HOLD = "hold"
    EXIT = "exit"


class TrendStrategy:
    """趋势跟踪策略（均线交叉策略）"""

    def __init__(self, data: pd.DataFrame, ma_short: Optional[int] = None, ma_long: Optional[int] = None):
        self.data = data.copy()
        self.signals = pd.Series(index=data.index, dtype=object)
        self.positions = pd.Series(index=data.index, dtype=int)
        self.current_position = 0
        self.ma_short = ma_short or MA_SHORT
        self.ma_long = ma_long or MA_LONG
        self.ma_short_col = f"MA{self.ma_short}"
        self.ma_long_col = f"MA{self.ma_long}"

    def generate_signals(self) -> pd.DataFrame:
        print("📡 开始生成交易信号...")

        if self.ma_short_col not in self.data.columns or self.ma_long_col not in self.data.columns:
            raise ValueError(f"数据中缺少必要的均线指标: {self.ma_short_col}, {self.ma_long_col}")

        self.signals = pd.Series(Signal.HOLD.value, index=self.data.index)
        self.positions = pd.Series(0, index=self.data.index)

        for i in range(1, len(self.data)):
            if pd.isna(self.data[self.ma_short_col].iloc[i]) or pd.isna(self.data[self.ma_long_col].iloc[i]):
                continue

            ma_short_today = self.data[self.ma_short_col].iloc[i]
            ma_long_today = self.data[self.ma_long_col].iloc[i]
            ma_short_yesterday = self.data[self.ma_short_col].iloc[i - 1]
            ma_long_yesterday = self.data[self.ma_long_col].iloc[i - 1]

            golden_cross = (ma_short_yesterday <= ma_long_yesterday) and (ma_short_today > ma_long_today)
            death_cross = (ma_short_yesterday >= ma_long_yesterday) and (ma_short_today < ma_long_today)

            if golden_cross:
                self.signals.iloc[i] = Signal.BUY.value
                self.current_position = 1
            elif death_cross:
                self.signals.iloc[i] = Signal.SELL.value
                self.current_position = 0
            else:
                self.signals.iloc[i] = Signal.HOLD.value

            self.positions.iloc[i] = self.current_position

        self.data["signal"] = self.signals
        self.data["position"] = self.positions
        self._analyze_signals()
        return self.data

    def _analyze_signals(self):
        signal_counts = self.signals.value_counts()

        print("📊 信号生成统计:")
        print(f"   - 总信号数: {len(self.signals)}")
        for signal, count in signal_counts.items():
            percentage = count / len(self.signals) * 100
            print(f"   - {signal}: {count} 次 ({percentage:.1f}%)")

        buy_signals = (self.signals == Signal.BUY.value).sum()
        sell_signals = (self.signals == Signal.SELL.value).sum()
        total_trades = min(buy_signals, sell_signals)

        print(f"   - 买入信号: {buy_signals} 次")
        print(f"   - 卖出信号: {sell_signals} 次")
        print(f"   - 潜在交易次数: {total_trades} 次")

    def get_signal_details(self) -> Dict:
        buy_signals = self.data[self.data["signal"] == Signal.BUY.value]
        sell_signals = self.data[self.data["signal"] == Signal.SELL.value]

        return {
            "buy_signals": {
                "count": len(buy_signals),
                "dates": buy_signals["date"].tolist() if "date" in self.data.columns else [],
                "prices": buy_signals["close"].tolist() if "close" in self.data.columns else [],
            },
            "sell_signals": {
                "count": len(sell_signals),
                "dates": sell_signals["date"].tolist() if "date" in self.data.columns else [],
                "prices": sell_signals["close"].tolist() if "close" in self.data.columns else [],
            },
            "hold_periods": self._calculate_hold_periods(),
        }

    def _calculate_hold_periods(self) -> List[Dict]:
        hold_periods = []
        in_position = False
        start_idx = None
        start_date = None
        start_price = None

        for i in range(len(self.data)):
            signal = self.signals.iloc[i]

            if signal == Signal.BUY.value and not in_position:
                in_position = True
                start_idx = i
                start_date = self.data["date"].iloc[i] if "date" in self.data.columns else i
                start_price = self.data["close"].iloc[i] if "close" in self.data.columns else None
            elif signal == Signal.SELL.value and in_position:
                in_position = False
                end_date = self.data["date"].iloc[i] if "date" in self.data.columns else i
                end_price = self.data["close"].iloc[i] if "close" in self.data.columns else None
                hold_days = i - start_idx
                return_pct = ((end_price - start_price) / start_price * 100) if start_price and end_price else None
                hold_periods.append(
                    {
                        "start": start_date,
                        "end": end_date,
                        "hold_days": hold_days,
                        "start_price": start_price,
                        "end_price": end_price,
                        "return_pct": return_pct,
                    }
                )

        if in_position and start_idx is not None:
            end_idx = len(self.data) - 1
            end_date = self.data["date"].iloc[end_idx] if "date" in self.data.columns else end_idx
            end_price = self.data["close"].iloc[end_idx] if "close" in self.data.columns else None
            hold_days = end_idx - start_idx
            return_pct = ((end_price - start_price) / start_price * 100) if start_price and end_price else None
            hold_periods.append(
                {
                    "start": start_date,
                    "end": end_date,
                    "hold_days": hold_days,
                    "start_price": start_price,
                    "end_price": end_price,
                    "return_pct": return_pct,
                    "is_open": True,
                }
            )

        return hold_periods


class SimpleStrategy:
    """简化版策略（仅生成信号，不管理持仓）"""

    @staticmethod
    def generate_simple_signals(data: pd.DataFrame) -> pd.DataFrame:
        data = data.copy()
        ma_short_col = f"MA{MA_SHORT}"
        ma_long_col = f"MA{MA_LONG}"

        if ma_short_col not in data.columns or ma_long_col not in data.columns:
            raise ValueError("数据中缺少必要的均线指标")

        data["signal"] = Signal.HOLD.value

        for i in range(1, len(data)):
            if pd.isna(data[ma_short_col].iloc[i]) or pd.isna(data[ma_long_col].iloc[i]):
                continue

            ma_short_today = data[ma_short_col].iloc[i]
            ma_long_today = data[ma_long_col].iloc[i]
            ma_short_yesterday = data[ma_short_col].iloc[i - 1]
            ma_long_yesterday = data[ma_long_col].iloc[i - 1]

            if (ma_short_yesterday <= ma_long_yesterday) and (ma_short_today > ma_long_today):
                data.loc[data.index[i], "signal"] = Signal.BUY.value
            elif (ma_short_yesterday >= ma_long_yesterday) and (ma_short_today < ma_long_today):
                data.loc[data.index[i], "signal"] = Signal.SELL.value

        return data
