"""
网格交易策略模块
"""
import pandas as pd
import numpy as np

class GridStrategy:
    """动态网格交易策略"""
    
    def __init__(self, data: pd.DataFrame, grid_count: int = 5, grid_step_pct: float = 0.05):
        self.data = data.copy()
        self.grid_count = grid_count
        self.grid_step_pct = grid_step_pct
        
    def generate_signals(self) -> pd.DataFrame:
        print(f"📡 生成网格交易信号 (网格数:{self.grid_count}, 步长:{self.grid_step_pct*100}%)...")
        self.data['signal'] = 'hold'
        self.data['signal_size'] = 0.0
        
        if len(self.data) == 0:
            return self.data
            
        base_price = self.data['close'].iloc[0]
        # 初始化网格线
        grid_levels = []
        for i in range(-self.grid_count, self.grid_count + 1):
            grid_levels.append(base_price * (1 + i * self.grid_step_pct))
        
        # 按价格从低到高排序
        grid_levels = sorted(grid_levels)
        
        # 记录当前价格在哪个网格区间
        def get_level_index(price):
            for idx, level in enumerate(grid_levels):
                if price <= level:
                    return idx
            return len(grid_levels)

        current_level = get_level_index(base_price)
        
        # 每次交易的仓位比例 = 1.0 / 网格总数 (大致估算)
        # 这里简化为每次动用1/grid_count的资金或股票
        trade_fraction = 1.0 / self.grid_count
        
        # 初始买入一半仓位作为底仓 (为了能卖)
        self.data.loc[self.data.index[0], 'signal'] = 'buy'
        self.data.loc[self.data.index[0], 'signal_size'] = 0.5
        
        for i in range(1, len(self.data)):
            price = self.data['close'].iloc[i]
            new_level = get_level_index(price)
            
            if new_level < current_level:
                # 价格下跌跨越网格线 -> 买入
                self.data.loc[self.data.index[i], 'signal'] = 'buy'
                self.data.loc[self.data.index[i], 'signal_size'] = trade_fraction
                current_level = new_level
                
            elif new_level > current_level:
                # 价格上涨跨越网格线 -> 卖出
                self.data.loc[self.data.index[i], 'signal'] = 'sell'
                self.data.loc[self.data.index[i], 'signal_size'] = trade_fraction
                current_level = new_level
                
        return self.data