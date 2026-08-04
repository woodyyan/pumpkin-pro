# 信号中心（Signal Center）设计文档

## 1. 问题理解

用户诉求是「盘中实时收到信号以便操作」，不是「收盘后看日报」。2026-08-03 代码核实结论：

- 盘中评估有效：`liveService.GetDailyBars` 走腾讯 fqkline 接口，盘中最后一根 bar 是当日形成中 K 线（close=最新价、volume=当日累计量），缓存 TTL 5 分钟，评估器 15 分钟 tick + `IsTradingHoursAt` 门控。盘中频率（15m/30m/1h）语义真实，保留。
- 真问题一：盘中信号基于未完成 K 线，收盘可能反转，但事件无「试算 vs 确认」语义。
- 真问题二：收盘后交易时段门控使评估直接跳过，日线策略标准的「收盘确认信号」永不生成。
- 真问题三：放量类信号盘中失真（累计成交量上午偏小，上午难触发、尾盘易误触发）。

设计方向：**保留盘中频率，新增信号双状态语义（盘中试算 / 收盘确认）+ 收盘确认评估**，而非删除盘中频率或改为纯收盘后评估。

## 2. 边界情况与失败场景

| 场景 | 处理 |
|---|---|
| 盘中信号收盘前反转 | 试算事件已落库不回撤，记录流标注「盘中试算」；收盘确认事件独立生成，用户可对比 |
| 同方向信号盘中反复触发（金叉→消失→再金叉） | 指纹含 trade_date+side+bar_state，同日同向同状态去重；cooldown 保留 |
| 停牌 | 快照/日线无新数据，价格提醒不触发；指标信号最后一根 bar 不更新，指纹去重天然抑制重复 |
| 非交易日 / 半日市 | 盘中 tick 按 IsTradingHoursAt 门控；收盘确认按市场交易日历调度，休市跳过该市场 |
| 腾讯行情源失败 | 沿用现有 ErrDataSourceDown 静默跳过 + stale 缓存；收盘确认评估失败记录日志，次日 catch-up 不补（信号时效性优先于完整性） |
| 策略被删除/停用但订阅仍引用 | 策略信号订阅显示「策略已失效」标记，评估跳过；不级联删除订阅 |
| 批量 scope 的自选股变化 | 评估时实时展开当时自选股列表，不做快照固化（P2） |
| 用户关闭订阅后仍有未读事件 | 事件保留可回看，仅停止新评估 |
| 迁移期间旧配置与新订阅并存 | 迁移一次性完成，迁移后旧表只读保留；旧 API 保留至个股页切换完成 |
| 港股台风休市 | 市场维度独立判断（沿用 quadrant worker 的市场日历模式），不连带跳过 A 股 |

## 3. 系统设计

### 3.1 模块划分

```
frontend/pages/signals.js            信号中心页面（五区结构）
frontend/lib/signal-center-ui.js     视图模型纯函数（状态归一、分组、徽章）
backend/store/signal/template.go     模板注册表（代码常量 + seed 校验）
backend/store/signal/subscription.go 订阅 CRUD + 迁移
backend/store/signal/evaluator.go    盘中评估（改造：写入 bar_state=intraday_provisional）
backend/store/signal/close_confirmer.go 收盘确认评估（新增 worker）
backend/store/signal/pricer.go       价格提醒评估（实时快照比对，新增）
backend/store/signal/recommender.go  规则推荐引擎（P1）
quant/main.py                        /api/signal/evaluate 复用（指标/策略类）
quant/signal/price_rules.py          价格提醒纯函数（新增，轻量）
```

### 3.2 模块职责

- **模板注册表（template.go）**：系统预置模板的唯一事实源。字段：key、name、category（price_alert / indicator / strategy）、implementation_key（映射 quant evaluator）、param_schema、default_params、intraday_mode、sort_order。代码常量维护，启动时 seed 校验，不做 Admin 可编辑（对齐 D-021）。
- **订阅（subscription.go）**：用户配置的事实表。一只股票可多个订阅（打破 user+symbol 唯一）。开关、参数覆盖、scope。
- **盘中评估器（evaluator.go 改造）**：保持现有 15 分钟 tick + 交易时段门控 + 频率门控；产出事件写入 bar_state=intraday_provisional。
- **收盘确认器（close_confirmer.go 新增）**：A 股 15:10、港股 16:10 触发（配置化，沿用 quadrant worker 的市场日历跳过模式）。此时腾讯日线最后一根 bar 已定型，评估结果写 bar_state=close_confirmed。与盘中评估器共享评估逻辑，仅调度与状态标记不同。
- **价格提醒评估（pricer.go）**：订阅的 price_alert 类模板不走 quant（无需 120 根日线），直接复用 live 实时快照（realtimeCacheTTL 10 秒）比对阈值，tick 1 分钟。涨破/跌破/单日涨跌幅三类。
- **推荐引擎（recommender.go，P1）**：输入 = 自选股 ∪ 持仓股 + 各股最新日线技术状态（复用 GetDailyBars + quant 指标）；规则映射输出（symbol, template_key, reason）。

### 3.3 数据流（盘中）

```
tick(15min)
  → ListEnabledSubscriptions
  → 按 scope 展开为 (user, symbol, template, params) 评估单元
  → price_alert? → pricer（实时快照比对）
  → indicator/strategy? → IsTradingHoursAt 门控 → 频率门控 → cooldown 门控
      → GetDailyBars（含形成中 bar）→ quant /api/signal/evaluate
      → BUY/SELL → RiskGate 持仓门控（不变：全量落库 + 推送门控）
      → EmitSignal(bar_state=intraday_provisional)
  → 通知路由：站内 unread+1（默认）→ webhook dispatcher（若配置）
```

### 3.4 数据流（收盘确认）

```
A股 15:10 / 港股 16:10（市场日历门控）
  → ListEnabledSubscriptions（indicator/strategy 类）
  → GetDailyBars（最后一根已定型）→ quant evaluate
  → BUY/SELL → RiskGate → EmitSignal(bar_state=close_confirmed)
```

同一交易日同一订阅可能产出试算 + 确认两条事件，语义不同，均需保留（胜率复盘口径：以 close_confirmed 为准，intraday_provisional 仅作时效参考）。

### 3.5 依赖关系

- 新体系依赖 live.Service（行情）、strategy.Service（策略信号解析）、quant（指标计算）、portfolio（RiskGate 持仓上下文）——依赖方向与现有一致，无新增反向依赖。
- RiskGate 三态决策、suppressed_reason 归档、合规免责声明内联机制原样保留（2026-07-31 决策，不可回归）。

## 4. 数据结构 & 接口

### 4.1 signal_templates（seed，代码常量）

| 字段 | 说明 |
|---|---|
| key | 唯一键，如 macd_cross / rsi_range / price_above / price_below / pct_change |
| name / description | 展示名与一句话说明 |
| category | price_alert / indicator / strategy |
| implementation_key | 映射 quant evaluator（price_alert 类为 backend 内置） |
| param_schema / default_params | JSON，前端表单 schema 驱动渲染 |
| intraday_mode | provisional_ok（盘中可试算）/ close_only（仅收盘确认）/ volume_adjusted（盘中量比修正，P2） |
| supported_scopes | [symbol] / [symbol, watchlist] |
| is_active / sort_order | 上架与排序 |

首期模板清单：price_above、price_below、pct_change（price_alert）；macd_cross、rsi_range、ma_breakout、bollinger_reversion、volume_breakout（indicator，复用现有 9 个 implementation_key 中的 5 个）；strategy（strategy 类，兼容存量）。

### 4.2 signal_subscriptions（新表）

| 字段 | 说明 |
|---|---|
| id / user_id | 主键 / 归属 |
| template_key | 关联模板 |
| strategy_id | 可空，仅 strategy 类模板使用 |
| scope_type | symbol（P0）/ watchlist（P2） |
| symbol | scope_type=symbol 时必填 |
| params_json | 覆盖模板 default_params 的增量 |
| is_enabled | 开关，即点即生效 |
| eval_interval_seconds / cooldown_seconds | 频率与冷却（price_alert 类忽略 eval_interval，固定 1 分钟 tick） |
| last_evaluated_at | 频率门控游标 |

唯一约束：(user_id, template_key, strategy_id, scope_type, symbol)，允许同股多模板。

### 4.3 signal_events（扩字段）

新增：subscription_id、template_key、bar_state（intraday_provisional / close_confirmed / test）、trade_date、is_read。
指纹升级：user+symbol+template+side+trade_date+bar_state 哈希，同日同向同状态去重。

### 4.4 后端 API

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | /api/signal/templates | 模板列表（含 param_schema，前端表单驱动） |
| GET | /api/signal/subscriptions | 我的订阅，按 symbol 分组返回 |
| POST | /api/signal/subscriptions | 创建订阅（template_key + scope + symbol + params） |
| PUT | /api/signal/subscriptions/{id} | 改参数/开关（开关即点即生效） |
| DELETE | /api/signal/subscriptions/{id} | 删除订阅（二次确认） |
| GET | /api/signal/events?symbol=&bar_state=&side=&limit= | 信号记录流 |
| GET | /api/signal/events/unread-count | 导航角标 |
| POST | /api/signal/events/mark-read | 全部/按股票标记已读 |
| GET | /api/signal/recommendations | 推荐列表（P1） |
| POST | /api/signal/subscriptions/quick-enable | 推荐一键开启（P1，幂等） |

旧 /api/signal-configs 系列保留只读兼容至个股页切换完成；webhook 系列 API 不变。

### 4.5 quant 接口

- /api/signal/evaluate 复用不改（backend 传 bars，形成中 bar 天然包含）。
- 价格提醒类不走 quant：backend pricer 纯函数比对实时快照（避免每订阅一次 HTTP 扇出）。
- P2 批量：POST /api/signal/evaluate-batch，一次请求评估多 (symbol, implementation_key, params) 组合。

## 5. 设计决策

### D1 保留盘中频率 + 新增双状态，而非删除频率或纯收盘后

- 用户核心诉求是盘中时效；代码核实证明盘中评估真实有效（形成中 K 线 + 5 分钟缓存）。
- 纯收盘后方案被用户明确否决；删除频率属于把「语义缺失」误判为「数据静态」。
- 备选「分钟级 K 线评估」：腾讯分钟线接口可用但指标语义漂移（15 分钟 MACD ≠ 日线 MACD），用户认知成本高，留作 P2+ 演进选项。

### D2 模板 + 订阅分离，免策略前置

- 模板是系统能力（代码常量），订阅是用户配置；策略信号降级为 template.category=strategy 的一种，存量迁移无损。
- 备选「自动为用户建默认策略」：污染策略库列表，删除策略时的引用检查（409）会更混乱，否决。

### D3 价格提醒走 backend 实时快照，不走 quant

- 价格比对无需 120 根日线，复用 live 快照（10 秒缓存）即可；避免每订阅每分钟一次 backend→quant HTTP 扇出。
- 口径边界：所有「指标计算」仍在 quant，pricer 只做阈值比对，不构成第二条指标链路。

### D4 收盘确认独立 worker，不复用盘中 tick

- 收盘确认要求「最后一根 bar 已定型」的时点语义，与盘中 tick 的频率门控、cooldown 语义不同；独立 worker 调度清晰，且与 quadrant/factorindex 的市场日历模式一致。
- 两 worker 共享评估纯函数，DRY 在逻辑层而非调度层。

### D5 通知默认站内，webhook 降级进阶

- 目标用户（持 1–2 股散户）配不了机器人 webhook；站内 unread + 角标是零配置默认通道。
- webhook 全链路（endpoints/deliveries/dispatcher）保留不动，仅 UI 从设置页迁入信号中心。

### D6 推荐一期规则引擎，不引 LLM

- 推荐输入（技术形态标签 + 持仓关系）是结构化数据，规则映射 deterministic、可解释、零成本；对齐 AI Picker「factorlab 候选池 + 规则约束」的减幻觉思路。
- 推荐记录带 reason 落库，便于复盘推荐质量后再评估 AI 增强（P2+）。

## 6. 扩展性设计

- **新增指标/策略信号**：quant registry 注册 implementation_key → template.go 加一条常量（param_schema 驱动前端表单，零前端定制）→ 完成。
- **新增信号品类**（如资金流信号、新闻事件信号）：template.category 加枚举值 + 对应 evaluator；评估调度按 intraday_mode 声明自动接入盘中/收盘链路。
- **批量信号**：scope_type=watchlist 已在模型层预留（supported_scopes、唯一约束含 scope_type），P2 仅需评估器展开逻辑 + UI 选择器。
- **通知通道**：通知路由层按 channel 分发，邮件（P2）、企业微信应用消息（远期）均为新增 channel 枚举 + sender 实现。
- **分钟级信号**：若未来引入，作为 intraday_mode=minute_bar 新模板品类，不动现有双状态语义。

## 7. UI/UX 设计

### 7.1 信息架构

- 导航「跟踪」组：自选股 / **信号中心** / 组合跟踪 / 持仓管理。信号中心带未读角标（复用 changelog badge 机制，badgeKey=signal）。
- `/signals` 单页五区纵排：概览条 → 推荐配置区 → 我的信号 → 信号记录 → 通知设置。
- 个股详情页「持仓 & 提醒」Tab 内信号区降级：该股票订阅状态摘要（N 个信号·最近触发）+「管理信号」跳转 /signals?symbol=XXX。
- 自选股页（P1）：每行操作区加「信号」入口，跳转 /signals?symbol=XXX。

### 7.2 PC 端

- 单栏最大宽度 1080px 居中，五区纵排；信息密度中等，卡片化。
- 「我的信号」按股票分组：每股票一张卡，卡头（名称/代码/最新价/涨跌）+ 卡内订阅列表（每行：模板名、参数摘要、状态徽章、开关、编辑、删除）。开关即点即生效（沿用现有交互偏好）。
- 信号记录流：表格/列表混合，行内徽章区分「盘中试算」（琥珀色）/「收盘确认」（蓝/红绿方向色）；筛选器（股票/方向/状态）置顶。
- 推荐配置区：横排卡片（股票、推荐模板、一句话理由、「一键开启」按钮）；已开启同模板则该卡消失。
- 新增订阅弹窗：搜股（复用 NavSearchBox 数据源）→ 模板选择（分类分组，参数表单 schema 驱动）→ 确认开启。
- 通知设置区：站内通道状态（默认开，仅展示）+ webhook 折叠面板（从设置页原样迁入）。

### 7.3 移动端

- 单列布局；概览条压缩为一行三指标；推荐区横滑卡片；其余区保持纵排。
- 订阅卡片内操作收敛：开关外露，编辑/删除收进行内展开区。
- 记录流简化为列表（去掉表格列），徽章与筛选保留；筛选器改为底部弹出层。
- 新增订阅弹窗改全屏抽屉（搜股键盘体验）。
- 弱化项：通知设置区的 webhook 配置在移动端只读展示，编辑引导至 PC（低频操作，控制移动端复杂度）。

### 7.4 关键交互流程

**新用户首次开启信号（核心路径）**：导航「跟踪→信号中心」→ 空状态展示推荐区（若已有自选/持仓）或「添加第一个信号」→ 搜股/点推荐卡 → 模板默认参数预填 → 「开启」→ 卡片出现在「我的信号」，开关已开 → 状态徽章「等待下次评估」。3 步内完成，无策略、无 webhook 前置。

**盘中接收信号**：评估触发 → 站内 unread+1 → 导航角标出现 → 用户进信号中心 → 记录流顶部新事件（「盘中试算」徽章 + 方向色 + 理由 + 免责声明）→ 点击标记已读。若配置了 webhook，同步推送（文案前缀【盘中试算】）。

### 7.5 状态与规范

- loading：各区独立 skeleton，不整页阻塞。
- empty：无订阅时推荐区充当主内容；无自选/持仓时推荐区展示「先添加自选股获得推荐」引导 + 手动添加入口。
- error：行情源失败时记录流正常展示历史，概览条标注「评估暂停，恢复后自动继续」。
- 颜色遵守语义化 token；方向色红涨绿跌；试算=琥珀（warning）、确认=信息蓝（BUY 红 / SELL 绿）。
- 合规：页面底部固定风险提示；每条事件内联免责声明（沿用现有机制）。

## 8. 风险评估

| 风险 | 等级 | 缓解 |
|---|---|---|
| 盘中试算信号收盘反转，用户误操作 | 高 | 双状态徽章 + 文案前缀 + 事件内联免责声明；胜率复盘以 close_confirmed 为准 |
| 腾讯行情扇出成本 | 中 | 订阅规模小（用户×1–2 股）；日线 5 分钟缓存、快照 10 秒缓存兜底；P2 批量接口削峰 |
| 事件表膨胀 | 中 | 同日同向同状态指纹去重 + cooldown；月度归档任务（沿用 backup 链路模式） |
| 迁移破坏存量策略信号 | 中 | 一次性迁移脚本 + 旧表只读保留 + 迁移前后行数/启用数对账；旧 API 保留至个股页切换 |
| 放量类信号盘中误触发 | 中 | P0 将 volume_breakout 标记 intraday_mode=close_only（仅收盘确认），P2 量比修正后放开 |
| RiskGate 回归 | 中 | 订阅→评估单元转换层保留 Config 语义；门控纯函数与 2 条防护测试原样运行 |
| 推荐规则误导 | 低 | reason 落库可复盘；推荐仅「一键开启」不自动开启，用户始终显式确认 |
| 双 worker 时钟漂移 | 低 | 收盘确认时间配置化；worker 启动 catch-up 只补当日，不跨日补发 |
