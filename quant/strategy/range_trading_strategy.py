"""
区间交易策略模块
"""
import pandas as pd

class RangeTradingStrategy:
    """区间交易策略 (基于RSI)"""
    
    def __init__(self, data: pd.DataFrame, rsi_period: int = 14, rsi_low: float = 30.0, rsi_high: float = 70.0):
        self.data = data.copy()
        self.rsi_period = rsi_period
        self.rsi_low = rsi_low
        self.rsi_high = rsi_high
        
    def generate_signals(self) -> pd.DataFrame:
        print(f"📡 生成区间交易信号 (RSI周期:{self.rsi_period}, 买入线:{self.rsi_low}, 卖出线:{self.rsi_high})...")
        self.data['signal'] = 'hold'
        self.data['signal_size'] = 1.0  # 全仓买卖
        
        rsi_col = f'RSI_{self.rsi_period}'
        if rsi_col not in self.data.columns:
            raise ValueError(f"缺失RSI指标 ({rsi_col})")
            
        for i in range(1, len(self.data)):
            if pd.isna(self.data[rsi_col].iloc[i]):
                continue
                
            rsi_today = self.data[rsi_col].iloc[i]
            rsi_yesterday = self.data[rsi_col].iloc[i-1]
            
            # RSI上穿低位线 -> 买入 (确认反弹)
            if rsi_yesterday <= self.rsi_low and rsi_today > self.rsi_low:
                self.data.loc[self.data.index[i], 'signal'] = 'buy'
                
            # RSI下穿高位线 -> 卖出 (确认回调)
            elif rsi_yesterday >= self.rsi_high and rsi_today < self.rsi_high:
                self.data.loc[self.data.index[i], 'signal'] = 'sell'
                
        return self.data