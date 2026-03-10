"""
回测系统配置文件
"""

# ===================================
# 数据配置
# ===================================
DATA_PATH = "data/stock.csv"  # 股票数据文件路径
DATE_COL = "date"             # 日期列名
PRICE_COLS = ["open", "high", "low", "close"]  # 价格列名
VOLUME_COL = "volume"         # 成交量列名

# ===================================
# 回测引擎配置
# ===================================
INITIAL_CAPITAL = 100000      # 初始资金（元）
TRANSACTION_FEE = 0.001       # 交易手续费率（0.1%）
SLIPPAGE = 0.0                # 滑点（暂不考虑）
EXECUTION_PRICE = "next_open" # 执行价格：next_open, same_close

# ===================================
# 策略参数配置
# ===================================
MA_SHORT = 20                 # 短期均线周期
MA_LONG = 60                  # 长期均线周期
ATR_PERIOD = 14               # ATR计算周期

# ===================================
# 输出配置
# ===================================
OUTPUT_PATH = "output/"       # 输出文件路径
PLOT_DPI = 100                # 图表DPI
SAVE_RESULTS = True           # 是否保存结果到文件
VERBOSE = True               # 是否输出详细日志