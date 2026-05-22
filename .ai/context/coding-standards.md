# Coding Standards

## source_trade_date 工程规范

### 后端

1. **所有日级收盘后批处理必须写入 `source_trade_date`**。写入值由交易日历服务查询"前一交易日"得到，不得用 `computed_at - 1 day` 推算。
2. **交易日历按市场区分**。A 股和港股交易日不完全一致（如港股有台风休市、A股有春节长假），`source_trade_date` 必须按对应市场独立计算。
3. **批次级字段**。同一批计算产出的所有结果共享同一 `source_trade_date`，作为批次元数据而非行级字段。
4. **API 响应必须包含 `source_trade_date`**。任何返回"日级收盘后数据"的 API 必须在顶层或批次级包含 `source_trade_date`，供前端渲染。
5. **禁止在用户口径中使用 `computed_at` 或 `snapshot_date` 替代 `source_trade_date`**。`computed_at` 仅用于内部运维日志和排障。

### 前端

1. **统一 helper 渲染日期文案**。所有用户可见的日期展示必须通过统一 helper 函数，格式为：`按 {source_trade_date} 收盘后数据生成`。
2. **禁止绕过 helper 直接使用 `computed_at`**。不得在其他模块中偷用 `computed_at` 做用户日期展示。
3. **Helper 入口单一**。项目内只维护一个日期渲染 helper，避免多个模块各自格式化导致口径不一致。

### 新模块接入清单

新模块如需展示"日级收盘后"数据日期，必须满足：

- [ ] 后端批处理写入 `source_trade_date`（按市场交易日历）
- [ ] API 响应包含 `source_trade_date`
- [ ] 前端使用统一 helper 渲染文案
- [ ] 不直接使用 `computed_at` 或 `snapshot_date` 作为用户口径日期
