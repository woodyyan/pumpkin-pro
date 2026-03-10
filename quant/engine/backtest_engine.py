"""
回测引擎模块 - 负责模拟交易
"""

import pandas as pd
import numpy as np
from typing import Dict

from config import EXECUTION_PRICE, INITIAL_CAPITAL, TRANSACTION_FEE


class BacktestEngine:
    """回测引擎"""

    def __init__(self, data: pd.DataFrame, initial_capital: float = None, fee: float = None):
        self.data = data.copy()
        self.initial_capital = initial_capital or INITIAL_CAPITAL
        self.fee_rate = fee or TRANSACTION_FEE
        self.execution_price = EXECUTION_PRICE

        self.cash = []
        self.shares = []
        self.portfolio_value = []
        self.trades = []
        self.daily_returns = []

        self.current_cash = self.initial_capital
        self.current_shares = 0
        self.current_position = 0
        self.current_date = None

    def run_backtest(self) -> pd.DataFrame:
        print("🚀 开始运行回测...")
        print(f"  初始资金: ¥{self.initial_capital:,.2f}")
        print(f"  手续费率: {self.fee_rate * 100:.2f}%")
        print(f"  执行价格: {self.execution_price}")

        self.cash = []
        self.shares = []
        self.portfolio_value = []
        self.trades = []
        self.daily_returns = []

        self.current_cash = self.initial_capital
        self.current_shares = 0
        self.current_position = 0

        required_cols = ["signal", "close", "open"]
        for column in required_cols:
            if column not in self.data.columns:
                raise ValueError(f"数据中缺少必要列: {column}")

        has_signal_size = "signal_size" in self.data.columns
        dates = self.data["date"].tolist() if "date" in self.data.columns else list(range(len(self.data)))

        for index in range(len(self.data)):
            self.current_date = dates[index]
            current_signal = self.data["signal"].iloc[index]
            current_size = self.data["signal_size"].iloc[index] if has_signal_size else 1.0

            if pd.isna(current_size) or current_size <= 0:
                current_size = 1.0 if not has_signal_size else 0.0

            current_close = self.data["close"].iloc[index]

            if self.execution_price == "next_open" and index < len(self.data) - 1:
                execution_price = self.data["open"].iloc[index + 1]
            else:
                execution_price = current_close

            if current_signal == "buy" and current_size > 0:
                self._execute_buy(execution_price, index, current_size)
            elif current_signal == "sell" and current_size > 0 and self.current_shares > 0:
                self._execute_sell(execution_price, index, current_size)

            current_value = self._calculate_portfolio_value(current_close)
            self.cash.append(self.current_cash)
            self.shares.append(self.current_shares)
            self.portfolio_value.append(current_value)

            if index == 0:
                daily_return = 0.0
            else:
                previous_value = self.portfolio_value[index - 1]
                daily_return = (current_value - previous_value) / previous_value if previous_value else 0.0
            self.daily_returns.append(daily_return)

        self.data["cash"] = self.cash
        self.data["shares"] = self.shares
        self.data["portfolio_value"] = self.portfolio_value
        self.data["daily_return"] = self.daily_returns
        self.data["cumulative_return"] = pd.Series(self.portfolio_value).div(self.initial_capital).sub(1.0)

        self._print_backtest_summary()
        return self.data

    def _execute_buy(self, price: float, index: int, size_pct: float = 1.0):
        if price <= 0:
            print(f"⚠️ 第{index}天: 买入价格无效 ({price})")
            return

        available_cash = self.current_cash * size_pct
        fee = available_cash * self.fee_rate
        investable_cash = available_cash - fee
        shares_to_buy = int(investable_cash / price)

        if shares_to_buy <= 0:
            return

        cost = shares_to_buy * price
        total_fee = cost * self.fee_rate
        total_cost = cost + total_fee

        self.current_shares += shares_to_buy
        self.current_cash -= total_cost
        self.current_position = 1

        self.trades.append(
            {
                "date": self.current_date,
                "type": "buy",
                "price": price,
                "shares": shares_to_buy,
                "amount": cost,
                "fee": total_fee,
                "cash_after": self.current_cash,
                "shares_after": self.current_shares,
            }
        )

    def _execute_sell(self, price: float, index: int, size_pct: float = 1.0):
        if self.current_shares <= 0:
            return

        if price <= 0:
            print(f"⚠️ 第{index}天: 卖出价格无效 ({price})")
            return

        shares_to_sell = int(self.current_shares * size_pct)
        if shares_to_sell <= 0:
            return

        revenue = shares_to_sell * price
        fee = revenue * self.fee_rate
        net_revenue = revenue - fee

        self.current_cash += net_revenue
        self.current_shares -= shares_to_sell
        if self.current_shares == 0:
            self.current_position = 0

        self.trades.append(
            {
                "date": self.current_date,
                "type": "sell",
                "price": price,
                "shares": shares_to_sell,
                "amount": revenue,
                "fee": fee,
                "cash_after": self.current_cash,
                "shares_after": self.current_shares,
            }
        )

    def _calculate_portfolio_value(self, current_price: float) -> float:
        return self.current_cash + self.current_shares * current_price

    def _print_backtest_summary(self):
        if len(self.portfolio_value) == 0:
            print("⚠️ 回测结果为空")
            return

        final_value = self.portfolio_value[-1]
        total_return = (final_value - self.initial_capital) / self.initial_capital * 100
        total_days = len(self.portfolio_value)
        years = total_days / 252
        annual_return = ((final_value / self.initial_capital) ** (1 / years) - 1) * 100 if years > 0 else 0.0
        max_drawdown = self._calculate_max_drawdown()
        total_trades = len(self.trades)
        buy_trades = len([trade for trade in self.trades if trade["type"] == "buy"])
        sell_trades = len([trade for trade in self.trades if trade["type"] == "sell"])

        print("\n" + "=" * 60)
        print("📊 回测结果摘要")
        print("=" * 60)
        print(f"初始资金:      ¥{self.initial_capital:,.2f}")
        print(f"最终资产:      ¥{final_value:,.2f}")
        print(f"总收益:        {total_return:+.2f}%")
        print(f"年化收益:      {annual_return:+.2f}%")
        print(f"最大回撤:      {max_drawdown:.2f}%")
        print(f"总交易次数:    {total_trades} 次")
        print(f"买入交易:      {buy_trades} 次")
        print(f"卖出交易:      {sell_trades} 次")
        print(f"回测天数:      {total_days} 天")

        if total_trades > 0:
            total_fee = sum(trade["fee"] for trade in self.trades)
            print(f"总手续费:      ¥{total_fee:,.2f}")

        print("=" * 60)

    def _calculate_max_drawdown(self) -> float:
        if not self.portfolio_value:
            return 0.0

        portfolio_series = pd.Series(self.portfolio_value)
        rolling_max = portfolio_series.expanding().max()
        drawdowns = (portfolio_series - rolling_max) / rolling_max * 100
        max_drawdown = drawdowns.min()
        return abs(max_drawdown) if max_drawdown < 0 else 0.0

    def get_trade_log(self) -> pd.DataFrame:
        if not self.trades:
            return pd.DataFrame()
        return pd.DataFrame(self.trades)

    def get_performance_metrics(self) -> Dict:
        if len(self.portfolio_value) == 0:
            return {}

        final_value = self.portfolio_value[-1]
        total_return_pct = (final_value - self.initial_capital) / self.initial_capital * 100
        total_days = len(self.portfolio_value)
        years = total_days / 252
        cagr = ((final_value / self.initial_capital) ** (1 / years) - 1) * 100 if years > 0 else 0.0

        daily_returns_series = pd.Series(self.daily_returns)
        sharpe_ratio = 0.0
        if daily_returns_series.std() > 0:
            sharpe_ratio = (daily_returns_series.mean() * 252) / (daily_returns_series.std() * np.sqrt(252))

        return {
            "initial_capital": self.initial_capital,
            "final_capital": final_value,
            "total_return_pct": total_return_pct,
            "annual_return_pct": cagr,
            "max_drawdown_pct": self._calculate_max_drawdown(),
            "total_trades": len(self.trades),
            "win_rate_pct": self._calculate_win_rate(),
            "sharpe_ratio": sharpe_ratio,
            "total_days": total_days,
            "total_fee": sum(trade["fee"] for trade in self.trades) if self.trades else 0,
            "avg_daily_return_pct": daily_returns_series.mean() * 100,
            "volatility_pct": daily_returns_series.std() * 100 * np.sqrt(252),
        }

    def _calculate_win_rate(self) -> float:
        if len(self.trades) < 2:
            return 0.0

        winning_trades = 0
        total_paired_trades = 0
        index = 0
        while index < len(self.trades) - 1:
            if self.trades[index]["type"] == "buy" and self.trades[index + 1]["type"] == "sell":
                buy_price = self.trades[index]["price"]
                sell_price = self.trades[index + 1]["price"]
                if sell_price > buy_price:
                    winning_trades += 1
                total_paired_trades += 1
                index += 2
            else:
                index += 1

        return (winning_trades / total_paired_trades) * 100 if total_paired_trades else 0.0
