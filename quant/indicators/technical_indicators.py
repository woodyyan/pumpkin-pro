"""
指标模块 - 负责计算技术指标
"""

import pandas as pd
import numpy as np
from typing import Optional, Dict, List
from config import MA_SHORT, MA_LONG, ATR_PERIOD


class TechnicalIndicators:
    """技术指标计算器"""
    
    def __init__(self, data: pd.DataFrame, price_col: str = "close"):
        """
        初始化指标计算器
        
        Parameters:
        -----------
        data : pd.DataFrame
            股票数据
        price_col : str
            价格列名，默认为"close"
        """
        self.data = data.copy()
        self.price_col = price_col
        
    def calculate_ma(self, period: int, price_col: Optional[str] = None) -> pd.Series:
        """
        计算移动平均线
        
        Parameters:
        -----------
        period : int
            移动平均周期
        price_col : str, optional
            价格列名，默认为self.price_col
            
        Returns:
        --------
        pd.Series
            移动平均线序列
        """
        col_name = price_col or self.price_col
        ma_series = self.data[col_name].rolling(window=period, min_periods=period).mean()
        return ma_series
    
    def calculate_atr(self, period: int = ATR_PERIOD) -> pd.Series:
        """
        计算平均真实波幅 (Average True Range)
        
        Parameters:
        -----------
        period : int
            ATR计算周期
            
        Returns:
        --------
        pd.Series
            ATR序列
        """
        # 计算真实波幅 (True Range)
        high = self.data['high']
        low = self.data['low']
        close = self.data['close'].shift(1)  # 前一日收盘价
        
        # TR = max(high - low, abs(high - close_prev), abs(low - close_prev))
        tr1 = high - low
        tr2 = abs(high - close)
        tr3 = abs(low - close)
        
        tr = pd.concat([tr1, tr2, tr3], axis=1).max(axis=1)
        
        # 计算ATR（简单移动平均）
        atr = tr.rolling(window=period, min_periods=period).mean()
        
        return atr
    
    def calculate_rsi(self, period: int = 14, price_col: Optional[str] = None) -> pd.Series:
        """
        计算 RSI (Relative Strength Index)
        
        Parameters:
        -----------
        period : int
            RSI计算周期
        price_col : str, optional
            价格列名，默认为self.price_col
            
        Returns:
        --------
        pd.Series
            RSI序列
        """
        col_name = price_col or self.price_col
        delta = self.data[col_name].diff()
        
        # 使用平滑移动平均线计算RSI更为准确
        gain = (delta.where(delta > 0, 0)).rolling(window=period).mean()
        loss = (-delta.where(delta < 0, 0)).rolling(window=period).mean()
        
        rs = gain / loss
        rsi = 100 - (100 / (1 + rs))
        
        # 处理 loss=0 的情况（RSI=100）
        rsi = rsi.fillna(100).where(loss != 0, 100)
        # 前 period 天设为 NaN
        rsi.iloc[:period] = np.nan
        
        return rsi

    def calculate_bollinger_bands(self, period: int = 20, std_dev: float = 2.0, price_col: Optional[str] = None):
        """
        计算布林带 (Bollinger Bands)
        
        Parameters:
        -----------
        period : int
            均线周期
        std_dev : float
            标准差倍数
        price_col : str, optional
            价格列名
            
        Returns:
        --------
        Tuple[pd.Series, pd.Series, pd.Series]
            (上限, 中轨, 下限)
        """
        col_name = price_col or self.price_col
        mid_band = self.data[col_name].rolling(window=period).mean()
        std = self.data[col_name].rolling(window=period).std()
        
        upper_band = mid_band + (std * std_dev)
        lower_band = mid_band - (std * std_dev)
        
        return upper_band, mid_band, lower_band

    def calculate_all_indicators(self) -> pd.DataFrame:
        """
        计算所有指标
        
        Returns:
        --------
        pd.DataFrame
            包含所有指标的数据
        """
        print("📈 开始计算技术指标...")
        
        # 计算MA20
        ma20 = self.calculate_ma(MA_SHORT, self.price_col)
        self.data[f'MA{MA_SHORT}'] = ma20
        print(f"✅ MA{MA_SHORT} 计算完成")
        
        # 计算MA60
        ma60 = self.calculate_ma(MA_LONG, self.price_col)
        self.data[f'MA{MA_LONG}'] = ma60
        print(f"✅ MA{MA_LONG} 计算完成")
        
        # 计算ATR（可选）
        try:
            atr = self.calculate_atr(ATR_PERIOD)
            self.data[f'ATR{ATR_PERIOD}'] = atr
            print(f"✅ ATR{ATR_PERIOD} 计算完成")
        except Exception as e:
            print(f"⚠️ ATR计算失败: {e}")
        
        # 计算金叉死叉信号
        self._calculate_crossover_signals()
        
        # 检查指标数据完整性
        self._validate_indicators()
        
        return self.data
    
    def _calculate_crossover_signals(self):
        """计算均线交叉信号"""
        ma_short_col = f'MA{MA_SHORT}'
        ma_long_col = f'MA{MA_LONG}'
        
        if ma_short_col not in self.data.columns or ma_long_col not in self.data.columns:
            return
        
        # 计算金叉（短期均线上穿长期均线）
        golden_cross = (self.data[ma_short_col] > self.data[ma_long_col]) & \
                      (self.data[ma_short_col].shift(1) <= self.data[ma_long_col].shift(1))
        
        # 计算死叉（短期均线下穿长期均线）
        death_cross = (self.data[ma_short_col] < self.data[ma_long_col]) & \
                     (self.data[ma_short_col].shift(1) >= self.data[ma_long_col].shift(1))
        
        self.data['golden_cross'] = golden_cross
        self.data['death_cross'] = death_cross
        
        # 统计交叉次数
        golden_count = golden_cross.sum()
        death_count = death_cross.sum()
        
        print(f"📊 均线交叉统计:")
        print(f"   - 金叉次数: {golden_count}")
        print(f"   - 死叉次数: {death_count}")
    
    def _validate_indicators(self):
        """验证指标数据"""
        indicator_cols = [f'MA{MA_SHORT}', f'MA{MA_LONG}']
        
        for col in indicator_cols:
            if col in self.data.columns:
                missing = self.data[col].isnull().sum()
                if missing > 0:
                    print(f"⚠️ {col} 有 {missing} 个缺失值（前{MA_LONG}天无法计算）")
    
    def get_indicator_summary(self) -> Dict:
        """
        获取指标摘要信息
        
        Returns:
        --------
        dict
            指标摘要
        """
        summary = {}
        
        # MA指标摘要
        ma_short_col = f'MA{MA_SHORT}'
        ma_long_col = f'MA{MA_LONG}'
        
        if ma_short_col in self.data.columns:
            summary[ma_short_col] = {
                "first_value": self.data[ma_short_col].dropna().iloc[0],
                "last_value": self.data[ma_short_col].dropna().iloc[-1],
                "mean": self.data[ma_short_col].mean(),
                "missing": self.data[ma_short_col].isnull().sum()
            }
        
        if ma_long_col in self.data.columns:
            summary[ma_long_col] = {
                "first_value": self.data[ma_long_col].dropna().iloc[0],
                "last_value": self.data[ma_long_col].dropna().iloc[-1],
                "mean": self.data[ma_long_col].mean(),
                "missing": self.data[ma_long_col].isnull().sum()
            }
        
        # 交叉信号摘要
        if 'golden_cross' in self.data.columns:
            summary['cross_signals'] = {
                "golden_cross": self.data['golden_cross'].sum(),
                "death_cross": self.data['death_cross'].sum()
            }
        
        return summary
    
    def plot_indicators_sample(self, n_days: int = 50):
        """
        绘制指标样本图（用于调试）
        
        Parameters:
        -----------
        n_days : int
            显示的天数
        """
        try:
            import matplotlib.pyplot as plt
            
            # 获取最后n_days的数据
            plot_data = self.data.tail(n_days).copy()
            
            fig, axes = plt.subplots(2, 1, figsize=(12, 8))
            
            # 价格和均线图
            ax1 = axes[0]
            ax1.plot(plot_data.index, plot_data[self.price_col], label='Close', linewidth=2)
            
            ma_short_col = f'MA{MA_SHORT}'
            ma_long_col = f'MA{MA_LONG}'
            
            if ma_short_col in plot_data.columns:
                ax1.plot(plot_data.index, plot_data[ma_short_col], label=f'MA{MA_SHORT}', alpha=0.7)
            
            if ma_long_col in plot_data.columns:
                ax1.plot(plot_data.index, plot_data[ma_long_col], label=f'MA{MA_LONG}', alpha=0.7)
            
            # 标记交叉点
            if 'golden_cross' in plot_data.columns:
                golden_points = plot_data[plot_data['golden_cross']].index
                ax1.scatter(golden_points, plot_data.loc[golden_points, self.price_col], 
                          color='green', s=100, marker='^', label='Golden Cross')
            
            if 'death_cross' in plot_data.columns:
                death_points = plot_data[plot_data['death_cross']].index
                ax1.scatter(death_points, plot_data.loc[death_points, self.price_col], 
                          color='red', s=100, marker='v', label='Death Cross')
            
            ax1.set_title(f'Price and Moving Averages (Last {n_days} Days)')
            ax1.set_xlabel('Index')
            ax1.set_ylabel('Price')
            ax1.legend()
            ax1.grid(True, alpha=0.3)
            
            # ATR图（如果有）
            atr_col = f'ATR{ATR_PERIOD}'
            if atr_col in plot_data.columns:
                ax2 = axes[1]
                ax2.plot(plot_data.index, plot_data[atr_col], label=f'ATR{ATR_PERIOD}', color='orange')
                ax2.set_title(f'ATR (Last {n_days} Days)')
                ax2.set_xlabel('Index')
                ax2.set_ylabel('ATR')
                ax2.legend()
                ax2.grid(True, alpha=0.3)
            
            plt.tight_layout()
            plt.show()
            
        except ImportError:
            print("⚠️ Matplotlib未安装，无法绘制图表")
        except Exception as e:
            print(f"⚠️ 绘图失败: {e}")


if __name__ == "__main__":
    # 测试指标模块
    print("🧪 测试指标模块...")
    
    # 创建示例数据
    from data.data_loader import create_sample_data, DataLoader
    
    # 加载数据
    loader = DataLoader()
    if not loader.data_path.exists():
        create_sample_data(str(loader.data_path))
    
    data = loader.prepare_data()
    
    # 计算指标
    indicator_calc = TechnicalIndicators(data)
    data_with_indicators = indicator_calc.calculate_all_indicators()
    
    # 输出摘要
    summary = indicator_calc.get_indicator_summary()
    print("\n📊 指标摘要:")
    for key, value in summary.items():
        print(f"  {key}: {value}")
    
    # 显示前10行数据
    print("\n📋 数据前10行（含指标）:")
    print(data_with_indicators.head(10))
    
    # 尝试绘图
    indicator_calc.plot_indicators_sample(100)