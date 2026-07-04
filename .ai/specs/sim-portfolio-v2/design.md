# Design: 模拟组合 v2 Calendar-driven Pipeline

## 总体设计

模拟组合 v2 使用 Calendar-driven Portfolio Pipeline：

```text
market_calendar
  -> pipeline day plan
  -> sim_portfolio_v2_signal_batches/items
  -> sim_portfolio_v2_selection_batches/items
  -> sim_portfolio_v2_price_requirements
  -> sim_portfolio_v2_daily/positions/trades/metrics
  -> verification
  -> /portfolio-tracking
```

核心原则：

- 市场交易日历决定“应该跑什么”。
- Pipeline 状态记录“实际跑了什么”。
- 严格模式保证“缺信号/缺价格/验证失败不生成正式收益”。
- Admin 操作 pipeline 阶段，不再直接修补旧字段。

## 模块划分

### 1. Market Calendar Service

职责：

- `isTradingDay(market, date)`
- `nextTradingDay(market, date)`
- `previousTradingDay(market, date)`
- `latestCompletedTradingDay(market, now)`

短期可由内置工作日日历 + 明确港股/A股假期表提供，后续可替换为外部交易日历源。

### 2. Pipeline Orchestrator

阶段：

- `calendar`
- `signal`
- `selection`
- `price_requirements`
- `entry_open`
- `valuation_close`
- `facts`
- `verify`

状态：

- `pending`
- `running`
- `ok`
- `skipped`
- `blocked`
- `failed`

### 3. Signal Snapshot Builder

从 `quadrant_ranking_snapshots` 为指定市场和 `source_trade_date` 构建模拟组合专用信号批次。信号批次必须检查数量、价格和交易日口径。

### 4. Portfolio Selection Engine

从信号批次按组合定义选出成分股。成分股不足时标记 `shortfall` 并阻断后续阶段。

### 5. Price Requirement Engine

为每个选股项生成：

- `entry_open`：信号日下一交易日开盘价。
- `valuation_close`：建仓交易日收盘价。

休市日不生成价格需求。

### 6. Price Resolver

按价格需求解析精确交易日价格：

- 开盘价使用 `OpenPriceResolver`。
- 收盘价使用 `PriceLookupResolver` 或 `PriceResolver`。
- 返回价格日期必须与需求交易日一致。

### 7. Fact Engine

所有价格需求满足后生成 v2 事实表，包括 daily、positions、trades、metrics。生成后执行 verification，通过后 public API 才可展示。

## 旧链路下线

下线旧 Admin 操作：

- 同步最新事实表
- 从头重算全部组合
- 验证事实表一致性
- 补齐建仓开盘价
- 补齐收盘价
- 设置全局开始信号日

旧表暂不 drop，但 v2 不读写旧表。

## UI 设计

### PC Admin

新增“模拟组合 Pipeline”独立区块：

- 顶部市场状态卡：A 股、港股。
- 日期矩阵：日期 × 阶段。
- 缺口诊断：显示 blocking reasons。
- 操作区：初始化 v2、运行 pipeline、刷新状态。
- 运行日志：最近 pipeline runs。

### Mobile Admin

移动端只展示：

- 市场状态卡。
- 严重阻断列表。
- 推荐操作按钮。
- 最近运行记录。

不展示完整矩阵和高级日期范围操作。
