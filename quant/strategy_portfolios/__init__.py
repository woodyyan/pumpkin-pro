"""卧龙组合看板（strategyportfolio）策略组合回放引擎。

与 quant/strategy/ 下的单票交易信号策略不同，本目录承载「多资产组合级」
独立回放引擎：输入价格矩阵与参数，输出每日净值 / 持仓 / 模拟交易 / 指标 /
数据质量报告，供 Go 后端物化为不可变 release。

首期唯一策略：C6 行业 ETF RS 轮动（c6_rs_rotation）。
"""
