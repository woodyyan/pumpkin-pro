# Design: 卧龙股池

## 架构

```text
factor_index_daily (latest per index)
  -> rebalance_id
  -> factor_index_constituents (rank <= 10)
  -> factorpool.Service
  -> live.Service.GetDetailedSnapshots (deduplicated, 10-second cache)
  -> GET /api/live/factor-pool
  -> /stock-pool
```

## 职责边界

| 模块 | 职责 |
|---|---|
| `factorindex.Repository` | 读取当前日度物化记录、关联调仓批次和 Top10 成分；不读取实时行情。 |
| `factorpool.Service` | 组合月度冻结事实和批量行情，维护行情失败的降级语义。 |
| `live.Service` | 获取并短缓存批量实时行情。 |
| `/stock-pool` | 渲染结果、每 10 秒刷新，不计算排名或因子值。 |

## 当前批次选择

每个指数的当前股池必须通过该指数最新 `factor_index_daily` 的 `rebalance_id` 确定，再读取对应 `factor_index_constituents`。这一约束确保指数日度值、`source_trade_date`、状态和成分都来自同一物化批次。

## 行情降级

- 行情是覆盖层：更新 `last_price`、`change_rate`、`turnover`、`quote_updated_at`。
- 因子排名、因子分、行业、调仓信号日均只来自因子事实表。
- 批量行情调用失败时接口仍返回完整榜单，`quote_status=unavailable`，行级行情字段为 `null`；不得用 0 伪造行情、不得删除成分股。

## 交互

- PC：七个卡片按响应式多列平铺，卡内紧凑行显示主字段。
- 移动端：因子标签横向滚动，一次仅展开一个榜单，减轻 70 行连续滚动。
- 点击股票进入既有 `/live-trading/[symbol]`。

## 测试

1. 后端：七榜固定顺序、每榜 Top10 截断、单次批量行情调用、行情失败降级。
2. 前端：响应归一化、页面 JSX、聚合接口调用、导航顺序和移动因子切换。
3. 回归：完整 Go 测试、完整前端测试和生产构建。
