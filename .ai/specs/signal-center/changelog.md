# 信号中心（Signal Center）变更记录

## 2026-08-04 P0 核心闭环落地（T1–T16）

### 后端

- 新增 `signal_templates` 注册表（`store/signal/template.go`，代码常量）：price_above / price_below / pct_change / macd_cross / rsi_range / ma_breakout / bollinger_reversion / volume_breakout（close_only）/ strategy 共 9 个模板，param_schema 驱动前端表单。
- 新增 `signal_subscriptions` 表与 CRUD（subscription.go / subscription_repository.go / subscription_service.go）：五元组唯一键 (user, template, strategy, scope, symbol)，一股可配多个信号；创建默认即开启（一键开启）；风控字段与旧配置同语义。
- `signal_events` 扩字段：subscription_id / template_key / bar_state（intraday_provisional / close_confirmed / realtime / test）/ trade_date / is_read；指纹升级为含 template_key+bar_state，同日同向不同状态可共存。
- 迁移（migrate.go）：旧 symbol_signal_configs 一次性迁移为策略信号订阅，幂等可重跑，旧表只读保留，启动时对账日志（legacy/enabled/created/skipped/failed）。
- 评估器改为订阅驱动（evaluator.go 重写）：盘中试算写 intraday_provisional；close_only 模板盘中跳过；cooldown 按订阅+bar_state 维度；RiskGate 经 riskConfigRecord 兼容层完整保留。
- 新增收盘确认 worker（close_confirmer.go）：A 股 15:10 / 港股 16:10（SIGNAL_CLOSE_CONFIRM_* 配置化），周末跳过，休市由「最后一根 bar 必须当日」隐含门控；启动 catch-up 当日。
- 新增价格提醒 pricer（pricer.go）：1 分钟 tick，复用 live 实时快照（新增 live.Service.GetDetailedSnapshots 透传），涨破/跌破/单日涨跌幅纯函数比对，事件 bar_state=realtime。
- 新 API：GET /api/signal/templates；GET/POST /api/signal/subscriptions；PUT/DELETE /api/signal/subscriptions/{id}；GET /api/signal/events（symbol/bar_state/side 过滤）；GET /api/signal/events/unread-count；POST /api/signal/events/mark-read。
- 旧 /api/signal-configs 转只读：GET 保留，PUT/DELETE 返回 410 引导新 API；POST /test 保留。
- 策略删除引用检查升级为旧配置+新订阅合并计数（CountSignalRefsByStrategy / ListSignalRefs）。
- 顺带修复（独立 bug）：analysis_history generateUUID 由时间戳位移构造，同一纳秒窗口产生重复 ID 且末 6 字节恒 0，改为 crypto/rand 真随机 UUID v4。

### 前端

- 导航「跟踪」组新增「信号中心」/signals（自选股之后），badgeKey=signal 未读角标（_app useSignalUnread 60s 轮询，进页面乐观清零 + mark-read 落库）。
- 新增 /signals 五区页面：概览条 / 空状态引导（P1 推荐占位）/ 我的信号（按股分组，开关即点即生效+编辑+删除）/ 信号记录（symbol/方向/状态筛选 + 试算/确认/实时徽章）/ 通知设置（站内默认 + webhook 折叠面板）。
- 新增订阅弹窗：搜股（/api/search）→ 模板分组选择 → schema 驱动参数表单 → 频率/高级风控 → 创建即开启；?symbol=XXX 跳入自动预选。
- 个股详情页信号区降级：本股订阅摘要 + 跳转 /signals?symbol=；旧内嵌配置编辑器下线（lib/signal-config-ui.js 及其测试保留为 legacy，后续清理）。
- 自选股页「信号」角标切换到 /api/signal/subscriptions。
- 设置页 webhook 两个 section 替换为「信号通知」链接卡；webhook 完整功能迁入 components/SignalWebhookPanel.js（信号中心内）。
- 新增 lib/signal-center-ui.js 纯函数库（17 个单测）+ signals-page.test.js（7 个单测）。

### 用户可见行为

- 无策略用户 3 步（选股票 → 选模板 → 开启）即可配置信号；站内提醒默认开启，导航角标提示未读。
- 盘中触发的事件标记「盘中试算」（含反转提示文案），收盘后独立生成「收盘确认」事件；价格提醒标记「实时触发」。
- 一只股票可同时开启多个信号；同股同向同日，盘中试算与收盘确认各自只触发一次。

### 已知遗留

- lib/signal-config-ui.js 与 signal-config-interaction.test.js / signal-config-ui.test.js 为 legacy 死代码（页面已不再引用），下个迭代清理。
- volume_breakout 仅收盘确认（intraday_mode=close_only），P2 量比修正后放开盘中试算。
- 事件表膨胀控制依赖指纹去重 + cooldown；月度归档任务未做（风险评估中已记录）。
