"""
均值回归策略模块
"""
import pandas as pd

class MeanReversionStrategy:
    """均值回归策略 (基于布林带)"""
    
    def __init__(self, data: pd.DataFrame, bb_period: int = 20):
        self.data = data.copy()
        self.bb_period = bb_period
        
    def generate_signals(self) -> pd.DataFrame:
        print(f"📡 生成均值回归信号 (布林带周期:{self.bb_period})...")
        self.data['signal'] = 'hold'
        self.data['signal_size'] = 1.0  # 全仓买卖
        
        # 需要确保数据中已经计算了布林带指标
        if 'BB_lower' not in self.data.columns or 'BB_upper' not in self.data.columns:
            raise ValueError("缺失布林带指标 (BB_lower, BB_upper)")
            
        for i in range(1, len(self.data)):
            if pd.isna(self.data['BB_lower'].iloc[i]) or pd.isna(self.data['BB_upper'].iloc[i]):
                continue
                
            close_today = self.data['close'].iloc[i]
            close_yesterday = self.data['close'].iloc[i-1]
            lower_band = self.data['BB_lower'].iloc[i]
            upper_band = self.data['BB_upper'].iloc[i]
            
            # 跌破下轨 -> 买入 (超卖反弹)
            if close_yesterday >= self.data['BB_lower'].iloc[i-1] and close_today < lower_band:
                self.data.loc[self.data.index[i], 'signal'] = 'buy'
                
            # 突破上轨 -> 卖出 (超买回调)
            elif close_yesterday <= self.data['BB_upper'].iloc[i-1] and close_today > upper_band:
                self.data.loc[self.data.index[i], 'signal'] = 'sell'
                
        return self.data