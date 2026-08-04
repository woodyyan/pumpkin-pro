# 信号中心（Signal Center）任务拆分

## P0 核心闭环

### 后端

- [ ] T1 模板注册表：`backend/store/signal/template.go` 代码常量定义首期 9 个模板（price_above / price_below / pct_change / macd_cross / rsi_range / ma_breakout / bollinger_reversion / volume_breakout[close_only] / strategy），启动 seed 校验。
- [ ] T2 `signal_subscriptions` 表 + CRUD service/repository；唯一约束 (user_id, template_key, strategy_id, scope_type, symbol)。
- [ ] T3 `signal_events` 扩字段：subscription_id / template_key / bar_state / trade_date / is_read；指纹升级为含 trade_date+bar_state。
- [ ] T4 迁移：旧 `symbol_signal_configs` → 策略信号订阅（一次性，旧表只读保留，迁移前后对账）。
- [ ] T5 订阅 API：templates 列表 / subscriptions CRUD / events 流 / unread-count / mark-read（main.go 路由，withRequiredAuth）。
- [ ] T6 盘中评估器改造：订阅展开（scope=symbol）、写入 bar_state=intraday_provisional；RiskGate 接入订阅语义（Config 兼容层）。
- [ ] T7 收盘确认 worker：A 股 15:10 / 港股 16:10（配置化 + 市场日历跳过），写 bar_state=close_confirmed；与盘中评估共享评估纯函数。
- [ ] T8 价格提醒 pricer：复用 live 实时快照，1 分钟 tick，涨破/跌破/单日涨跌幅三类纯函数比对。

### 前端

- [ ] T9 导航：`lib/navigation.js`「跟踪」组加「信号中心」/signals（自选股之后），badgeKey=signal 未读角标。
- [ ] T10 `/signals` 页面五区：概览条 / 推荐占位（P0 静态引导）/ 我的信号（按股分组卡）/ 信号记录（筛选+双状态徽章）/ 通知设置（webhook 迁入）。
- [ ] T11 新增订阅弹窗：搜股 → 模板选择（schema 驱动参数表单）→ 开启；开关即点即生效。
- [ ] T12 个股详情页信号区降级：订阅摘要 + 跳转 /signals?symbol=；旧完整配置 UI 下线，旧 /api/signal-configs 转只读兼容。
- [ ] T13 `lib/signal-center-ui.js` 纯函数 + 单测（状态归一、分组、徽章、去重展示）。

### 验证

- [ ] T14 `cd backend && go test ./...`（订阅 CRUD、迁移对账、双状态指纹、收盘确认调度、pricer 比对）。
- [ ] T15 `cd frontend && npm test && npm run build`。
- [ ] T16 端到端手工验收：无策略用户 3 步开启信号；盘中试算事件带标记；收盘确认事件独立生成；同股多信号各自触发。

## P1 推荐与联动

- [ ] T17 规则推荐引擎 recommender.go：输入自选股 ∪ 持仓 + 技术状态标签，规则映射（持仓→保护型 / 自选未持仓→机会型 / 趋势→均线+MACD / 震荡→RSI+布林 / 高波动→价格提醒），reason 落库。
- [ ] T18 GET /api/signal/recommendations + POST quick-enable（幂等）。
- [ ] T19 推荐配置区真实数据接入；空状态第一触点；已开启同模板推荐卡消失。
- [ ] T20 自选股页每行「信号」入口 → /signals?symbol=。

## P2 批量与触达增强

- [ ] T21 scope_type=watchlist：评估器展开当时自选股列表 + UI 批量配置选择器。
- [ ] T22 quant /api/signal/evaluate-batch 批量评估接口，降低扇出。
- [ ] T23 邮件通知通道（复用 mail provider）；通知路由按 channel 分发。
- [ ] T24 volume_breakout 盘中同时段量比修正，intraday_mode 升级为 volume_adjusted 后放开盘中试算。

## 依赖与顺序

- T1→T2→T3 是数据层底座，先行；T4 迁移在 T6 前完成（评估器只认订阅表）。
- T6/T7/T8 可并行；T9–T13 前端在 T5 API 就绪后并行。
- T17 推荐引擎依赖 T3 事件语义稳定；T21 依赖 T6 评估单元抽象。
