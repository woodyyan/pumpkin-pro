# 项目架构

## 技术栈
- **前端**: Next.js 14.2 + React 18 + Tailwind CSS 3.4
- **后端**: Go (Gin 框架)
- **数据**: SQLite (pumpkin.db)
- **量化引擎**: Python (quant/)

## 前端架构

### 目录结构
```
frontend/
├── components/     # 可复用组件
├── lib/            # 工具库、状态管理
├── pages/          # Next.js 页面路由
├── styles/         # 全局样式
├── public/         # 静态资源
├── data/           # 静态数据 (changelog.json)
└── __tests__/      # 测试
```

### 状态管理
- **AuthContext** (`lib/auth-context.js`): 用户认证状态
- **ThemeContext** (`lib/theme-context.js`): 浅色/深色主题状态
- 其他工具库 (lib/*.js): 无状态纯函数模块

### 样式体系 (2026-05-26 重构)
- **CSS 变量驱动**: 所有颜色通过 CSS 自定义属性定义
- **语义化 Token**: Tailwind `colors` 引用 CSS 变量
  - `background`, `background-alt`, `card`
  - `foreground`, `foreground-muted`, `foreground-dim`, `foreground-disabled`
  - `border`, `border-strong`
  - `primary`, `positive`, `negative`
- **主题切换**: `darkMode: "class"` 模式，通过 `<html class="dark">` / `<html class="light">` 控制
- **FOUC 防护**: `_document.js` 注入阻塞脚本，在 HTML 解析前设置 class

### 路由页面
| 路径 | 文件 | 说明 |
|---|---|---|
| `/` | `index.js` | 首页/落地页；内容配置集中在 `frontend/data/homepage.js`，主叙事为 AI投研工作台，包含 Hero、三大卖点、快速上手、功能分类、教程和风险提示 |
| `/live-trading` | `live-trading.js` | 市场行情页（导航归属「看板」）；当前实现为 `Hero + 单因子指数卡片 + 核心指数卡片 + 市场摘要` 的 dashboard，总览 A 股单因子指数与 A/H 股大盘指数 |
| `/live-trading/[symbol]` | `live-trading/[symbol].js` | 个股详情页 (194KB)，active 仍归属「看板 / 市场行情」 |
| `/ai/analysis` | `ai/analysis.js` | AI分析占位页（一期「敬请期待」） |
| `/ai/reports` | `ai/reports.js` | AI研报页面：A 股/中国香港股票个股研报介绍、缩略图样例、登录后弹窗预览、微信定制转化与合规风险提示 |
| `/ai/picker` | `ai/picker.js` | AI选股占位页（一期「敬请期待」） |
| `/ai/backtest` | `ai/backtest.js` | AI回测占位页（一期「敬请期待」） |
| `/quadrant` | `quadrant.js` | 四象限与卧龙AI精选页面（导航归属「看板 / 市场全景」），展示风险机会分布和精选榜单 |
| `/capital-map` | `capital-map.js` | 资金星图页面（导航归属「看板 / 资金星图」），基于 A 股东方财富公开行情样本展示 PE、成交额、板块资金流和 PoC 估值锚；前端 60 秒刷新，后端 30 秒内存缓存 |
| `/watchlist` | `watchlist.js` | 自选股页面 |
| `/stock-picker` | `stock-picker.js` | 选股器 |
| `/backtest` | `backtest.js` | 回测引擎 |
| `/strategies` | `strategies.js` | 策略库 |
| `/portfolio` | `portfolio.js` | 持仓管理 |
| `/factor-lab` | `factor-lab.js` | 因子实验室 |
| `/settings` | `settings.js` | 用户设置 |
| `/admin` | `admin.js` | 管理后台总览页（独立布局） |
| `/admin/data` | `admin/data.js` | 管理后台数据作业页（公司资料、因子流水线、四象限） |
| `/admin/ai` | `admin/ai.js` | 管理后台 AI 管理页（模型配置、AI 调用、AI 选股） |
| `/admin/ops` | `admin/ops.js` | 管理后台运维与支持页（支付、备份、系统健康、反馈） |
| `/changelog` | `changelog.js` | 更新日志 |
| `/share/ai-analysis-preview` | `share/ai-analysis-preview.js` | AI分析分享图预览 (独立布局) |

### 导航架构（2026-06-12）
- 主站导航不再在 `_app.js` 内硬编码多套数组，而是统一收敛到 `frontend/lib/navigation.js`。
- PC 端导航由 `components/DesktopNavMenu.js` + `components/NavDropdown.js` 渲染：hover 展开下拉，点击一级导航只负责展开/收起，不做跳转。
- 移动端导航由 `components/MobileNavMenu.js` 渲染：保留汉堡入口，菜单内部按「卧龙AI / 看板 / 跟踪 / 选股 / 更多」分组折叠，一次只展开一个分组。
- 「卧龙AI」分组当前顺序为「AI分析 / AI研报 / AI选股 / AI回测」。
- 「看板」分组当前顺序为「市场行情 / 市场全景 / 资金星图」。其中资金星图路由固定为 `/capital-map`，首期仅支持 A 股。
- 占位页统一复用 `components/ComingSoonPage.js`，文案固定为标题 + 「敬请期待」，避免散落多个空白实现。

### 首页内容架构（2026-06-27）
- 首页内容不再直接散落在 `frontend/pages/index.js`，而是集中维护在 `frontend/data/homepage.js`。
- 首页核心卖点固定为「AI 投研闭环 / 因子驱动选股 / 组合跟踪与复盘」。
- 快速上手采用任务型路径：AI分析、AI研报、AI选股、因子实验室、组合跟踪、持仓管理。
- 功能总览按「AI投研 / 市场与机会发现 / 选股与策略研究 / 跟踪与组合管理 / 账户服务」分类展示，功能项需带状态标签。
- AI分析、AI研报、AI选股、因子排序、策略回测、模拟组合、交易信号相关首页文案必须保留风险提示，不得暗示收益承诺。

### 页面模式补充（2026-06-13 / 2026-06-30）
- `frontend/pages/live-trading.js` 已从单行指数文本列表升级为 dashboard 页面，内部自行完成：
  - 指数标准化：`normalizeIndex(...)`
  - 页面分组：`buildMarketState(...)`
  - 市场摘要：`buildMarketInsights(...)`
- 页面新增 `frontend/lib/live-factor-index.js`，专门负责单因子指数卡片状态归一化；前端只请求 `/api/live/factor-index/overview` 并展示，不在页面内计算 NAV 或区间收益。
- 管理后台数据页新增 `frontend/components/admin/FactorIndexAdminPanel.js`，对应 `GET /api/admin/factor-index/status` 与 `POST /api/admin/factor-index/recompute`；后台只触发后端 worker / service 做异步补算和状态查询，不在前端直接重算。
- 趋势图组件继续复用 `frontend/components/MiniChart.js`，当前用于市场指数真实趋势点和单因子指数近 20 日净值曲线的轻量表达，不承载专业分时分析交互。
- 页面首屏新增 7 张 A 股单因子指数卡片，固定放在「核心指数卡片」上方；不新增单独详情页，用户在同一页完成因子指数与大盘指数对比。
- 页面首屏核心指数固定为 6 张：上证指数、深证成指、创业板指、恒生指数、恒生中国企业指数、恒生科技指数；扩展指数仍预留沪深300、科创50、上证50、中证500。

### 关键组件
- `ThemeToggle.js`: 主题切换按钮（三态：浅色/深色/跟随系统）
- `NavSearchBox.js`: 导航栏股票搜索
- `DesktopNavMenu.js`: 桌面端一级导航与 hover 下拉
- `MobileNavMenu.js`: 移动端分组折叠菜单
- `NavDropdown.js`: 桌面端二级导航下拉面板
- `ComingSoonPage.js`: 占位页通用组件
- `QuadrantChart.js`: 四象限风险图表
- `CapitalMapDashboard.js`: 资金星图页面主组件，负责 A 股 PE×成交额星图、板块资金图、PoC 分布、60 秒刷新和浅色/深色图表配色
- `RankingPanel.js` / `RankingPortfolioPanel.js`: 排行榜
- `PortfolioAttributionSection.js`: 持仓归因分析
- `AIAnalysisReportContent.js`: AI分析报告内容
- `MiniChart.js`: 轻量 canvas 迷你图，目前已复用于 admin 面板和市场行情页卡片

### Admin 页面架构（2026-06-16）
- 管理后台不再由单个超长 `frontend/pages/admin.js` 承载所有面板，而是拆为：
  - `frontend/components/admin/AdminShell.js`: 统一壳层，负责 session 校验、退出登录、PC 左侧导航、移动端抽屉导航、旧 `?tab=` 参数兼容跳转。
  - `frontend/components/admin/AdminSections.js`: 各 admin 面板与分组页面组件。
  - `frontend/components/admin/navigation.js`: admin 分组导航配置与旧 tab 到新路由的映射。
  - `frontend/pages/admin.js` / `frontend/pages/admin/data.js` / `frontend/pages/admin/ai.js` / `frontend/pages/admin/ops.js`: 路由入口。
- `_app.js` 对 admin 路由的独立布局判断已从“仅 `/admin`”扩展为“所有 `/admin*` 路径”，避免嵌套路由误套主站导航。
- admin 数据改为页面级按需拉取：
  - 总览页只拉用户/流量/设备/漏斗相关接口。
  - 数据作业页只拉公司资料、因子流水线、四象限相关接口。
  - AI 管理页只拉 AI 配置、AI 使用统计、AI研报、AI 选股相关接口；其中 AI研报管理使用 `/api/admin/ai-reports` 和 `/api/admin/ai-report-service-config` 管理 COS 图片 key 与微信服务配置；AI Picker 运维面板会并行请求 `/api/admin/ai-picker/status` 与 `/api/admin/ai-picker/latest-run`，分别承载“最近 10 条日志/最近结果摘要”和“最近一次完整 LLM 交互详情（system prompt、user prompt、provider reasoning、原始返回）”。
  - 运维与支持页只拉支付、备份、系统健康、反馈相关接口。

## 后端持仓快照架构补充（2026-06-02）

- `backend/store/portfolio/service.go` 中的历史快照能力已拆分为“历史快照重建引擎 + 单日/区间输出”两层：
  - 区间重建：用于全量历史回灌与删除后缓存重建。
  - 单日重建：用于分市场定时任务、`pnl-calendar` 当前月缺失补写、后续 CLI 历史补写。
- 历史重建的数据源固定为：
  - `user_portfolio_events`
  - `security_profiles`
  - 历史日线 `GetDailyBars(...)`
- `user_portfolio_daily_snapshots` 保存市场级聚合结果，`user_portfolio_position_daily_snapshots` 保存持仓级结果；两者都由同一重建引擎生成，避免市场级与持仓级口径分叉。
- `persistDailySnapshots(...)` 仍存在，但职责被限制为 dashboard / equity curve 的“当天轻量刷新”，不能再承担任何历史日期补写职责。
- `backend/store/portfolio/worker.go` 提供调度适配层：
  - A 股调度：北京时间 16:00。
  - 港股调度：北京时间 17:00。
  - 两条调度链路最终都调用 `Service.RunDailyMarketSnapshot(...)`，由服务层负责任务日志、用户遍历、单用户重建与汇总状态。
- `backend/cmd/rebuild-portfolio-daily-snapshots/main.go` 提供人工入口：
  - 市场级模式：调用 `RunDailyMarketSnapshot(...)` 执行整市场历史单日重建。
  - 用户级模式：调用 `RebuildDailySnapshotForUser(...)` 执行指定 `user + scope + date` 重建。
- 配置层新增 `PortfolioSnapshotConfig`，把开关、A 股/港股触发时间与超时统一放在 `backend/config/config.go` 管理，避免调度参数散落在 `main.go` 或 worker 常量中。

## 新口径模拟组合事实表运维链路（2026-07-01）

- `/portfolio-tracking` 只读取新事实表：`portfolio_daily`、`portfolio_position`、`portfolio_trade`、`portfolio_metrics`；legacy `quadrant_ranking_portfolio_results` 仅用于历史核查。
- Admin 四象限页的「新口径模拟组合管理」固定按三步表达：
  1. 「补齐建仓开盘价」只补 `quadrant_ranking_portfolio_market_prices.open_price` / `entry_trade_date`，不生成持仓或调仓。
  2. 「同步最新事实表」负责推进 `signal_date -> trade_date`，生成 `portfolio_daily/position/trade/metrics`。
  3. 「验证事实表一致性」检查资产汇总、持仓数量和建仓价完整性。
- Admin status 必须同时暴露前置价格缺口与事实表完整度：`daily_row_count`、`completed_daily_count`、`position_row_count`、`trade_row_count`、`metrics_row_count`、`baseline_only`、`can_sync`、`action_hint`。
- `SyncSimPortfolios` 返回每个组合的同步摘要：锚点、最新信号、生成估值日数量、最后生成日期和阻断原因；bulk-save 后的自动同步日志至少记录 generated 数量。
