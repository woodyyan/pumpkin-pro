# Design: 卧龙 AI 精选模拟组合指标改造

## 总体方案

采用“收盘后单组合表现页”方案：

1. 继续沿用收盘价模拟调仓和收益统计。
2. 页面主叙事从“跑赢基准”切换为“组合自身表现与风险画像”。
3. 后端结果表仍保留组合定义里的 benchmark 标识字段，便于兼容历史定义和 A/HK 默认指数信息，但不再计算、返回或展示 benchmark 收益曲线与超额收益结果。

## 前端设计

### 1. 顶部指标区

使用 7 个轻量指标卡：
- 累计收益
- 成立天数
- 昨日收益率
- 本月收益率
- 最大回撤
- 波动率
- 日胜率

设计约束：
- 维持单层卡片，不增加二级解释块。
- 成立天数卡片下方直接补“起始日 YYYY-MM-DD”。
- 风险指标使用中性色，收益类指标继续使用涨跌色。

### 2. 曲线区

- 只渲染组合累计收益曲线。
- 保留 0 收益基线，帮助用户判断盈亏区间。
- tooltip 展示：日期、累计收益、单日收益、回撤。

### 3. 成分股区

每行展示：
- 股票名 / 代码
- 如果是 B 组合，展示榜单名次与连续上榜天数
- 买入价
- 最新价
- `较买入价` 收益率
- 仓位

### 4. 最近一次调仓

继续保留折叠式 disclosure，不扩大默认信息密度。

## 后端设计

### 1. 结果构建

`buildRankingPortfolioResult` / 重建脚本统一改为只依赖：
- snapshots
- constituents
- market prices

不再依赖 benchmark price 表，也不再计算：
- benchmark nav
- benchmark return
- excess return
- daily benchmark return

### 2. 新增汇总指标

由收益序列推导：
- `inception_trade_date`
- `inception_days`
- `latest_daily_return_pct`
- `current_month_return_pct`
- `max_drawdown_pct`
- `volatility_pct`
- `daily_win_rate_pct`

### 3. 当前成分股补充字段

运行时 enrich 当前成分股：
- `entry_trade_date`
- `entry_price`
- `latest_trade_date`
- `latest_close_price`
- `latest_return_pct`

计算方式：
1. 先根据当前组合定义的历史 snapshots 找出该股票当前连续持有周期的起点 `source_trade_date`。
2. 用 `RankingSnapshot.price_trade_date` 查询该股票在起点日及最新日向前最近可用的收盘价。
3. 用两者计算 `latest_return_pct`。

### 4. 重建命令

`backend/cmd/rebuild-ranking-portfolio-results` 同步去 benchmark 化：
- 不再加载 benchmark 行情
- 不再读写 benchmark price 表
- 不再生成 benchmark 相关结果字段
- dry-run / write 日志改为只输出组合收益、成分股数和曲线长度

## 数据模型影响

### 保留

- `RankingPortfolioDefinition.BenchmarkCode`
- `RankingPortfolioDefinition.BenchmarkName`
- `RankingPortfolioSnapshot.BenchmarkCode`
- `RankingPortfolioSnapshot.BenchmarkName`
- `RankingPortfolioResult.BenchmarkCode`
- `RankingPortfolioResult.BenchmarkName`

原因：
- 兼容已有组合定义和历史快照
- 不需要额外迁移即可上线
- 未来如需展示“所属市场默认指数”标签，仍有元信息可用

### 删除/停用

删除/停用收益计算链路：
- `RankingPortfolioBenchmarkPrice`
- `latest_benchmark_nav`
- `latest_benchmark_return`
- `latest_excess_return_pct`
- `benchmark_nav`
- `benchmark_return_pct`
- `excess_return_pct`
- `daily_benchmark_return_pct`

说明：
- 当前版本已移除代码路径与结果输出。
- 历史数据库中的旧列/旧表即使仍存在，也不再参与 ranking portfolio 读写与展示。

## 测试设计

### 后端

1. 组合结果构建后不再输出 benchmark 指标。
2. 新增 summary metrics 单测：成立天数、本月收益率、最大回撤、波动率、日胜率。
3. 新增成分股 enrichment 单测：连续持有周期起点、买入价、最新价、较买入价收益。
4. 重建命令单测确认 HK/A 组合仍能生成计划，但不再依赖 benchmark 表。
5. 重建链路与普通保存链路都要回归通过。

### 前端

1. 组件测试校验顶部新指标、`较买入价` 文案与单曲线叙事。
2. helper 测试校验序列标准化仍能处理 drawdown 和单日收益。
3. 主题测试继续保证图表浅/深色主题可用。

## 风险与回滚

### 风险

1. 历史数据库中仍可能留有 benchmark 表和旧字段，不影响读取，但新链路不再使用。
2. 若 `price_trade_date` 覆盖不足，个别成分股可能无法补全“较买入价”收益。
3. 去 benchmark 后，旧前端缓存若仍期待双曲线数据，需要确保新前端同时上线。

### 回滚

- 代码层面可直接回滚到旧版本。
- 数据层面本次不做 destructive migration，旧表保留，因此回滚风险较低。
