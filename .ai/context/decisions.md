# 设计决策记录

## 决策 14: Stripe 本地钱包二期继续走 admin-only Hosted Checkout 单方式测试

**日期**: 2026-06-11

**背景**: Stripe 一期已经在 `/admin` 落地 card-only 的 Hosted Checkout 测试链路。二期需要把支付宝和微信支付也接进来，但用户明确要求仍然只做 admin 内测，不改公开站点，不把普通用户流量卷入本地钱包验证。

**决策**:
1. 支付宝与微信支付二期继续复用既有 `payments + payment_events + admin API + Stripe Hosted Checkout + webhook` 架构，不新增公开站点支付入口。
2. admin 每笔测试单只允许选择一种主支付方式（`card` / `alipay` / `wechat_pay`），不在同一笔 Checkout Session 内混合多个本地钱包做灰盒测试。
3. 当前阶段只支持 **一次性 payment mode**，不进入 subscription / setup 模式。
4. 微信支付在创建 Checkout Session 时必须显式设置 `payment_method_options.wechat_pay.client=web`，以适配 PC admin 场景的二维码扫码链路。
5. admin 前端的支付方式下拉、币种列表和测试提示由后端返回的支付方式元数据驱动，避免前后端各自硬编码一套能力口径。

**原因**:
1. Hosted Checkout 已经在一期验证过审计链路和 webhook 状态机，复用它比为本地钱包重做一套自定义前端更稳。
2. 单笔只测一种主支付方式，能让“为什么 Checkout 没显示该方式 / 为什么创建失败”这类问题更容易定位，不会被 fallback card 噪音干扰。
3. 支付宝与微信支付在 Stripe Checkout 下都更适合单次支付验证；若现在混入订阅，会把本地钱包接入问题和 recurring 模型问题耦合在一起。
4. 微信支付 PC 测试是二维码扫码链路，不补 `client=web` 会让 session 参数与预期测试形态不一致。
5. 能力元数据由后端下发后，前端才能稳定显示“是否启用、支持币种、跳转/扫码提示”，避免文案和真实配置漂移。

**替代方案**:
- 在同一笔 session 同时塞入 `card + alipay + wechat_pay`：否决，admin 内测需要可诊断性，混合多方式只会让 eligibility 和展示问题更难解释。
- 二期直接切到自定义前端 / PaymentIntents：否决，当前目标是先验证 Stripe 本地钱包链路，Hosted Checkout 变量更少、回归范围更小。
- 直接把本地钱包拉进 subscription 方案：否决，Stripe 对支付宝/微信支付的 recurring 能力和限制与 card 差异很大，应该后续单独设计。

**影响**:
- `backend/store/payment` 需要显式建模 admin 主支付方式、支持币种、Hosted Checkout flow 和 method-specific options。
- `/api/admin/payments/config` 需要返回 admin 可测试支付方式元数据，供 `/admin` 面板驱动选择器和提示。
- `/admin` 支付面板需要支持按支付方式切换币种与测试提示，并继续沿用“只跟踪当前本地测试单”的轮询策略。

---

## 决策 13: resolveEntryTradeDateForSnapshot 加今日北京时间兜底

**日期**: 2026-06-11

**背景**: `BackfillMissingEntryOpenPrices` 通过 `resolveEntryTradeDateForSnapshot` 推断 T+1 买入日期，逻辑是查 `snapshot_date > 当前快照日` 的后继 snapshot。D0（切换当天）以及港股只有一条 snapshot 时，不存在后继 → `ErrRecordNotFound → entryTradeDate="" → StillPendingCount++`，openPriceResolver 根本不被调用，所有股票永远 pending。

**决策**: 当找不到后继 snapshot 且 `todayBJ > snapshotDate` 时，以今日北京日期作为 T+1 的兜底值。当 `snapshotDate == todayBJ` 时不触发兜底（T+1 尚未到来，entry date 无法确定）。

**原因**:
1. 无后继 snapshot = 这是最新一批，下一个交易日就是今天（北京时间），这是合理的数据驱动推断
2. `snapshotDate < todayBJ` 的守卫确保不会用今天的日期来填充「未来的」entry date
3. 对港股 D0 这种只有一条 snapshot 的情况，能自动处理，无需特殊逻辑

**替代方案**:
- 继续依赖后继 snapshot — 否决，D0 和单条 snapshot 场景永远不可补齐
- 在 admin repair 里手动传入 entry date — 否决，增加操作复杂度，且依赖人工干预

**影响**: `portfolio_service.go resolveEntryTradeDateForSnapshot`；原有测试 `TestBackfillMissingEntryOpenPrices_PendingWhenNoSuccessorSnapshot` 更新为使用 `snapshotDate=todayBJ` 以保持原有语义；新增两个回归测试

---

## 决策 1: 浅色/深色模式方案

**日期**: 2026-05-26

**背景**: 网站默认整体黑色 UI，用户反馈阅读困难，需要支持浅色模式。

**决策**: 采用 CSS 变量 + Tailwind class 模式

**原因**:
1. Tailwind `dark:` 前缀方案需要每处写两套 class，~1500 处硬编码维护成本爆炸
2. CSS `filter: invert()` 会导致图表/图片色彩反转，不可控
3. CSS 变量方案一次定义全局生效，后续新增页面自动支持

**替代方案**:
- Tailwind `dark:` 前缀 — 否决，代码量翻倍
- CSS `filter: invert()` — 否决，效果差

**影响**: 
- `tailwind.config.cjs` 中 colors 全部改为 `var(--xxx)` 引用
- 新增 4 个文件，修改 41 个文件
- 引入 7 个语义化 token 替代 1500+ 硬编码值

## 决策 2: darkMode: "class" vs "media"

**决策**: 使用 `"class"` 模式

**原因**: `"media"` 仅跟随系统偏好，无法手动切换。`"class"` 允许程序化控制，支持手动 + 系统混合。

## 决策 3: FOUC 防护

**决策**: 在 `_document.js` 注入阻塞 `<script>`

**原因**: 服务端渲染无法获取 `localStorage`，首次加载会出现主题闪烁。阻塞脚本在 HTML 解析前完成 class 设置。

## 决策 4: 保留部分硬编码值

**保留**: `bg-black/70`（模态遮罩）、`bg-white/40`（视觉指示点）

**原因**: 这些颜色在深浅模式下都需要保持一致，不应随主题变化。

## 决策 5: 卧龙 AI 精选排行榜涨幅口径

**日期**: 2026-05-28

**背景**: 行情看板的卧龙 AI 精选排行榜同时展示"连续上榜 N 日"与"上榜以来"涨幅。历史实现将涨幅起点设为股票历史首次进入排行榜的快照日期，导致断档后重新上榜时，用户看到"连续上榜 1 日"但涨幅却来自更早历史批次。

**决策**: 排行榜 `return_pct` 必须按"当前连续上榜周期"计算。连续周期从最新榜单快照向前逐日连续追溯；若最新 2026-05-28 且连续 1 天，则起点就是 2026-05-28。

**原因**:
1. UI 上"连续上榜"和"上榜以来"应使用同一时间窗口，避免用户误读为单日或当前周期涨幅异常。
2. 历史首次入选涨幅属于另一个分析口径，不适合作为当前榜单主指标。
3. 断档重新入选应重置表现统计，否则旧批次价格会污染当前建议的解释。

**替代方案**:
- 保留历史首次入选涨幅并改文案为"首次入选以来"：否决，仍会弱化当前榜单周期含义。
- 同时展示历史首次入选涨幅和连续周期涨幅：暂不采用，增加 UI 信息密度；如未来需要，应显式拆分字段。

**影响**:
- `RankingItem.return_pct` 表示当前连续上榜周期以来涨幅。
- 后端应从当前连续周期起点快照价格算到最新可用快照价格。
- 历史快照价格错误需要通过专用回填命令修正；业务 API 不应在读取时自动篡改历史快照。

## 决策 6: 因子实验室行业口径统一为申万一级行业

**日期**: 2026-05-28

**背景**: 因子实验室列表长期显示“行业”空值。历史 `factor_security_industries` 只保存抓到的原始行业名称，未形成稳定的消费者口径；`company_profiles` 也缺少批量标准化链路，导致 Phase 2 写入 `factor_scores.industry` 时覆盖率极低。

**决策**: 用户侧统一展示 `company_profiles.industry_name`，且该字段只承载“申万一级行业”。`factor_security_industries` 保留为原始来源分层表；`industry_mapping` 保存来源行业到申万一级行业的维护映射。港股统一写入 `not_applicable`。

**原因**:
1. 只保留一个用户口径字段，避免 `factor_security_industries.industry_name` 与 `company_profiles.industry_name` 双维护。
2. 申万一级行业适合作为因子实验室筛选、列表展示和后续统计的稳定维度。
3. 港股与申万行业体系天然不一致，强行映射会制造错误分类，显式 `not_applicable` 更可控。

**影响**:
- Phase 0 新增 `industries` 模式：先刷新原始行业，再标准化写入 `company_profiles` 和 `industry_mapping`。
- Phase 2 读取行业时优先使用 `company_profiles.industry_name`，回退到 `factor_security_industries`。
- 每日 21:00 的 Factor Lab 自动预计算必须包含 `industries` 步骤。

## 决策 7: 组合日快照历史补写必须统一走事件重建链路

**日期**: 2026-06-02

**背景**: 第一阶段新增了分市场定时快照、`pnl-calendar` 缺失补写和后续 CLI 历史重建需求。实施中发现原有 `persistDailySnapshots(...)` 依赖实时持仓与实时报价，只能正确生成“今天”的视图；若继续用于历史日期，会把历史某日错误地按当前持仓写入快照。

**决策**: 历史日期的组合日快照与持仓日快照，统一通过“交易事件 + 历史行情”重建；`persistDailySnapshots(...)` 仅保留给 dashboard / equity curve 的当天轻量刷新。

**原因**:
1. 历史快照要求按 `user + scope + snapshotDate` 精确复原当日持仓状态，实时持仓路径天然不满足。
2. 定时任务、查询补写、CLI 三条链路若各自实现，会导致口径漂移和重复逻辑。
3. 统一重建引擎后，可以把幂等、元信息、任务日志和测试覆盖集中在单一路径维护。

**替代方案**:
- 继续复用 `persistDailySnapshots(...)` 并附加日期参数：否决，数据源仍是当前持仓，历史结果必然失真。
- 分别为 scheduler / query / CLI 写三套逻辑：否决，维护成本高且容易出现口径不一致。

**影响**:
- `RebuildDailySnapshotForUser(...)` 必须调用历史重建引擎。
- `GetPnlCalendar(...)` 的当前月缺失补写写入结果也必须采用历史口径。
- 分市场 worker 与 CLI 必须继续复用同一套历史重建服务，而非重新拼装实时持仓快照。

## 决策 8: 组合日快照调度按市场拆分固定北京时间窗口

**日期**: 2026-06-02

**背景**: 第一阶段只实现 A 股与港股两个市场的日快照补写。用户已明确要求 A 股在北京时间 16:00 后触发，港股在北京时间 17:00 后触发；同时 worker 与手动 CLI 都要复用已落地的历史单日重建服务。

**决策**: 新增独立 `portfolio.Worker`，内部按 `ASHARE` / `HKEX` 分两条调度循环，分别在 16:00 / 17:00 触发 `RunDailyMarketSnapshot(...)`；手动 CLI 只负责参数解析与服务装配，最终调用同一服务入口。

**原因**:
1. 市场闭市时间不同，若共用单一触发时刻，会让其中一个市场在价格尚未稳定时过早落快照。
2. 调度与手动入口都下沉到 `RunDailyMarketSnapshot(...)`，才能保证任务日志、幂等写入、失败统计和后续扩展维持同一口径。
3. 将调度状态与最近执行结果收敛到 worker 内部，便于后续接管理接口而不污染服务层领域逻辑。

**替代方案**:
- 单一 worker 固定在一个时间点串行处理两个市场：否决，无法严格满足不同市场闭市后的触发要求。
- CLI 直接拼装仓储与重建细节：否决，会绕开已有任务日志和服务层校验，导致口径分叉。

**影响**:
- `backend/config/config.go` 新增 `PortfolioSnapshotConfig`，对调度开关、A 股/港股触发时间和超时参数集中配置。
- `backend/main.go` 启动时会装配 `portfolioWorker`，与 `portfolioService` 并行存在。
- `backend/cmd/rebuild-portfolio-daily-snapshots` 成为第一阶段历史单日快照的标准人工重建入口。

## 决策 9: 注册失败提示必须返回字段级、可操作的反馈

**日期**: 2026-06-05

**背景**: 用户注册时，邮箱已存在、邮箱格式错误、密码为空、密码过短等不同失败场景，历史上都可能落到统一的 `invalid auth input`。前端只能展示笼统错误，用户不知道该改邮箱、改密码，还是直接去登录或找回密码，注册漏斗因此产生不必要阻塞。

**决策**: 注册链路必须把常见输入失败拆成稳定的字段级错误语义，并向前端返回明确错误码与可执行文案。最少覆盖：`EMAIL_REQUIRED`、`INVALID_EMAIL`、`PASSWORD_REQUIRED`、`PASSWORD_TOO_SHORT`、`EMAIL_EXISTS`。前端注册弹窗需要基于这些错误码给出针对性引导，而不是原样展示通用 `invalid input`。

**原因**:
1. 用户阻塞点集中在“下一步该做什么”不清楚，字段级反馈比通用报错更能直接降低流失。
2. 后端先稳定错误语义，前端才能安全地做错误映射、按钮跳转和实时引导，不必依赖字符串猜测。
3. `EMAIL_EXISTS` 不只是报错，还应引导用户直接登录或跳到找回密码，这是注册场景里的关键分流动作。

**替代方案**:
- 继续返回统一 `INVALID_INPUT`，只优化前端文案：否决，前端无法可靠区分邮箱已存在、邮箱格式错误和密码问题。
- 后端返回结构化字段数组，前端逐字段渲染：本阶段不采用，当前认证弹窗结构较轻，先用稳定错误码覆盖主要阻塞路径，复杂校验可后续再扩展。

**影响**:
- `backend/store/auth` 需要保留稳定错误类型，`backend/main.go` 负责映射为用户可读的 `code + detail`。
- `frontend/lib/auth-context.js` 的注册弹窗需要在提交前做最小必要校验，并对后端错误码做友好映射。
- 注册表单内应直接展示密码长度要求，并把“推荐同时包含字母和数字”作为引导而非硬性失败条件，避免不必要挡路。

## 决策 10: 卧龙 AI 精选模拟组合改为「T+1 开盘价建仓、当日收盘价估值」口径

**日期**: 2026-06-07

**背景**: 行情看板的卧龙 AI 精选模拟组合此前用榜单当日收盘价同时充当选股、建仓与估值价格（`close_price_rebalance`），存在时点错配/未来函数嫌疑：榜单一出（T 收盘后）就等于知道了买入价。业务要求改为更贴近真实交易的时序——T 收盘后选股，次一交易日 9:25 集合竞价开盘价模拟买入，当日收盘价结算收益与曲线。

**决策**:
1. 引入三条独立交易日口径：`signal_date`(T，选股依据收盘日) / `entry_date`(T+1，开盘建仓日) / `valuation_date`(收盘估值日)。
2. 买入价 = `entry_date` 的 9:25 集合竞价开盘价（`RankingPortfolioMarketPrice.OpenPrice` + `EntryTradeDate`）。
3. 净值曲线日收益改为等权 `open→close`（同一估值日开盘买入、收盘估值），不再用 `prevClose→curClose`。series 起点(T 收盘) NAV=1、日收益 0、不计涨跌。
4. 交易成本仅在买入腿/卖出腿发生（等权 + 单边 0.02%），连续在仓换手率为 0、不重复扣（复用 `calculateRankingPortfolioTradeRatio`）。
5. 个股「涨幅」= 实时价 / 开盘买入价 − 1。实时价每半小时刷新（`RankingPortfolioRealtimePrice` 缓存表 + `RealtimeWorker`），按北京时间分市场时点表：A 股 09:25/盘中半小时/15:30（共 12 点），港股 09:25/盘中半小时含12:00/16:30（共 15 点）。
6. 「昨日收益率」固定取「最新结算交易日的前一天」(T-1)，即 `series[len-2]`，且至少需 3 个 series 点。
7. 累计收益/最大回撤/波动率/日胜率/本月收益率算法不变，仅输入口径随 open→close 改变。
8. 历史数据采用 **cut-over**：抛弃历史曲线，从新算法上线日 D0 起 NAV=1 重新计算，不回算历史。

**原因**:
1. open→close + 三日口径消除了「榜单一出即知买入价」的时点错配，可解释性强。
2. 历史 `market_prices` 从未存过开盘价，9:25 集合竞价价无法从日线精确复原；老 close→close 与新 open→close 拼接会在切换点产生人为跳变，比断点更误导。cut-over 工作量小、口径一致、无跳变。
3. 实时价仅服务于「当前成分股涨幅」展示，与净值曲线（只用收盘价结算）物理隔离，避免曲线盘中乱跳。

**替代方案**:
- 继续用收盘价建仓：否决，时点错配、有未来函数嫌疑。
- 全量回算历史开盘价曲线：否决（仅保留为 plan B），数据不可校验、ROI 低、引入口径分叉。
- 引入状态机/版本号管理推荐组合 vs 已建仓组合：否决，榜单只在每日盘后定时刷新、盘中不变，无需状态机。

**影响**:
- `RankingPortfolioMarketPrice` 增 `OpenPrice`/`EntryTradeDate`；新增 `RankingPortfolioRealtimePrice` 表与 migrator 注册。
- 常量 `CalculationMethod=open_entry_close_valuation`、`PriceBasis=open_entry`、`MethodNote` 改写；`RebalanceRule` 沿用 `t_close_generate_t1_open_rebalance`。
- `calculateRankingPortfolioPeriodReturn` 改为单日 open→close；`buildRankingPortfolioResult` series 循环对应调整；`buildRankingPortfolioLatestRebalance` 参考价改用 `OpenPrice`。
- `enrichRankingPortfolioCurrentConstituents` 买入价取开盘价、最新价取实时价，开盘价未到时 `entry_price_pending=true`、涨幅置空、绝不用收盘价兜底。
- `buildRankingPortfolioSummaryMetrics` 昨日收益率取 T-1。
- Meta 增 `signal_date`/`entry_date`/`realtime_as_of`；ConstituentItem 增 `entry_price_pending`/`latest_price`/`latest_quote_time`。
- 新增 `RealtimeWorker`（北京时间分市场时点表）+ `config.RankingPortfolioRealtimeConfig`，在 `main.go` 装配并注入基于 `live.MarketClient.FetchDetailedSymbolSnapshots` 的实时报价 fetcher。
- 前端 `RankingPortfolioPanel.js` 展示开盘买入价、实时最新价、待开盘 pending 态，并更新口径说明文案。
- `cmd/rebuild-ranking-portfolio-results` 标注为旧 close→close 口径（plan B 历史近似回算），不作为新口径标准重建路径。
- `entry_date(T+1)` 由「下一个有行情快照的交易日」数据驱动推导（系统无独立节假日表）。

---

## 决策 12: FillRankingPortfolioEntryOpenPrice 去掉 snapshot_version 约束

**日期**: 2026-06-11

**背景**: D0 清库后 snapshots 表为空，`FillRankingPortfolioEntryOpenPrice` 原逻辑先查 snapshots 取 latest.SnapshotVersion，表为空时 ErrRecordNotFound → continue，导致所有 open_price 永远写不进去，成分股全天 pending。

**决策**: 去掉 `snapshot_version` 过滤，直接按 `definition_id + code + exchange + open_price<=0` UPDATE market_prices，不依赖 snapshots 表。

**原因**:
1. `open_price <= 0` 条件已保证幂等（已填的行不会被重复写）
2. market_prices 行在快照写入时已正确绑定 definition_id，不需要再绕道 snapshots 做版本隔离
3. 任何 snapshot_version 的行都应该被填入开盘价，约束 snapshot_version 只会在 D0 等边界场景下造成漏填

**替代方案**:
- 保留 snapshot_version，改用子查询 — 否决，仍依赖 snapshots 表非空，边界场景同样失败

**影响**: `repository.go FillRankingPortfolioEntryOpenPrice`，新增测试 `TestFillRankingPortfolioEntryOpenPrice_WorksWithEmptySnapshots`

---

## 决策 11: Admin「一键补齐」按钮改造为两阶段开盘价修复

**日期**: 2026-06-10

**背景**: 上线后若某日 09:25 实时 worker 未能成功回填开盘价（网络故障、服务中断等），对应 `market_prices.open_price` 仍为 0，导致该交易日无法结算收益曲线。需要运维手段补齐。

**决策**: 将 admin 后台「一键补齐」按钮职责从「补 close→close 曲线」改造为**两阶段开盘价修复**：
1. **Phase 1（开盘价回填）**: 扫描 `snapshot_date >= D0` 的 `market_prices` 缺 `open_price` 行，通过 `OpenPriceResolver`（`live.MarketClient.FetchSymbolDailyBars` 取 DailyBar.Open）精确匹配 T+1 日开盘价，幂等写入；
2. **Phase 2（曲线重算）**: 调用 `RebuildLaggingRankingPortfolioResultsFromDate(cutoverDate)`，从 D0 起重算缺失的收益曲线，含 D0 守卫（`fromDate` 不得早于 cutoverDate）。

**D0 守卫原则**: 任何修复路径的 `fromDate` 均不得早于 `RANKING_PORTFOLIO_CUTOVER_DATE`（默认 `2026-06-10`），防止误回算旧口径历史数据。

**OpenPriceResolver 设计**:
- 类型：`OpenPriceResolver func(ctx, code, exchange, tradeDate) float64`
- 精确匹配 `tradeDate`（T+1 日），不做日期 fallback。使用错日期的 open 会导致收益计算错误，宁缺毋错。
- 通过 `service.SetOpenPriceResolver(r)` 注入，按钮场景专用，与盘中实时 worker 解耦。

**接口变化**:
- `POST /api/admin/ranking-portfolio-repair` → 返回 `{ok, message, summary: {cutover_date, backfill_filled, backfill_still_pending, backfill_skipped_before_cutover}}`
- `GET /api/admin/ranking-portfolio-status` → 每条 item 新增 `cutover_date`, `pending_open_price_count` 字段

**前端变化**:
- 按钮文案：「一键补齐模拟组合收益曲线」→「补齐开盘价并重算曲线（仅上线日后）」
- 增加二次确认弹窗（点击后先显示确认行+确认/取消按钮，再执行）
- 执行后回显 summary（回填条数、待确认条数）
- 状态表格新增「开盘价缺口」列（缺 N 条 / 已补全），`pending > 0` 高亮 amber

**替代方案**:
- 方案 2：复用旧 `RebuildLaggingRankingPortfolioResults` 不改 fromDate：否决，无 D0 守卫会触及旧口径数据
- 方案 3：让 close price resolver 兼做 open 兜底：否决，收盘价 ≠ 开盘价，错误数据比缺数据更危险，pending 状态保持语义清晰

**影响文件**:
- `backend/config/config.go`：`RankingPortfolioRealtimeConfig` 加 `CutoverDate` 字段（env: `RANKING_PORTFOLIO_CUTOVER_DATE`，默认 `2026-06-10`）
- `backend/store/quadrant/service.go`：新增 `OpenPriceResolver` 类型、`SetOpenPriceResolver`、`TriggerRankingPortfolioRepairWithResult`、`GetRankingPortfolioAdminStatusWithCutover`
- `backend/store/quadrant/portfolio_service.go`：新增 `BackfillMissingEntryOpenPrices`、`resolveEntryTradeDateForSnapshot`、`RebuildLaggingRankingPortfolioResultsFromDate`（含 D0 守卫）
- `backend/store/quadrant/repository.go`：新增 `ListMarketPricesMissingOpenByDateRange`、`SetRankingPortfolioMarketPriceOpen`、`CountMissingOpenPricesByDefinition`
- `backend/store/quadrant/portfolio_model.go`：`RankingPortfolioAdminStatusItem` 加 `CutoverDate`/`PendingOpenPriceCount`/`LastRepairSummary`；新增 `RankingPortfolioRepairSummary`
- `backend/handlers_admin.go`：`handleAdminRankingPortfolioStatus`/`handleAdminRankingPortfolioRepair` 接入 cutoverDate 与新返回字段
- `backend/main.go`：新增 `newQuadrantOpenPriceResolver`，注入 `quadrantService.SetOpenPriceResolver`
- `frontend/pages/admin.js`：按钮改造 + 二次确认 + summary 回显 + 状态表「开盘价缺口」列

## 决策 14: Factor Lab industries 失败改为 warning 放行，dividends 改为手动刷新

**日期**: 2026-06-11

**背景**: 因子实验室 Phase 0 里 `industries` 长期只依赖 AkShare，且被视为阻断步骤；上游偶发失败时会直接让整条流水线停在 Phase 0。与此同时，`dividends` 每晚全量逐股刷新耗时长、更新频率却很低，容易把夜间自动链路拖慢甚至超时。

**决策**:
1. `industries` 自动源顺序改为 `akshare -> eastmoney -> tencent`。
2. `industries` 外部刷新失败时，Phase 0 只产出 `partial/warning`，不得再 hard fail 全流水线；Phase1/Phase2 必须继续执行。
3. admin 状态页必须展示 `industries` 的最近状态、warning、覆盖率、最近成功刷新时间。
4. `dividends` 从 nightly Phase 0 默认模式中移除，改为 admin 手动触发；保留“缺口修复”和“全量刷新”两个手动入口。

**原因**:
1. 行业字段对筛选和展示重要，但短时上游波动不应卡死整条因子链路。
2. 行业失败时最重要的是暴露健康度和覆盖率，让运维可见，而不是把 Phase1/Phase2 一起拖死。
3. 分红数据天然低频，放入 nightly 全量刷新既浪费窗口，也放大外部源慢请求风险。

**替代方案**:
- 继续把 `industries` 设为 critical: 否决，单点外部源波动会反复导致最新快照停滞。
- 仅把 `industries` 改成非 critical 但不展示健康度: 否决，运维无法区分“放行但健康”与“放行但已严重退化”。
- 保留 `dividends` nightly 自动全量刷新: 否决，收益低且直接占用 nightly 时间预算。

**影响**:
- `quant/scripts/backfill_factor_lab_phase0.py` 支持 industries 多源 fallback，并在失败时记录 warning summary。
- `quant/scripts/update_factor_lab_phase0_incremental.py` 默认 nightly modes 不再包含 `dividends`，但允许手动 `full_refresh_dividends`。
- `backend/store/factorlab/worker.go` 和 admin query/status 需要支持 `partial` 状态、dividends 手动 scope、industries 健康元数据。
- `frontend/pages/admin.js` 新增 industries 健康卡片、dividends 手动全量刷新按钮与策略说明。
