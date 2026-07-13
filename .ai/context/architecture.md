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

## CI / CD 构建发布链路（2026-07-02）

- GitHub Actions 当前分为 `build`、`publish_images`、`deploy` 三段：先做语言级检查，再统一构建并推送 3 个镜像，最后通过 SSH 在目标机执行 `docker compose pull && docker compose up -d`。
- Phase 1 起，`publish_images` 仍保持单 job 串行构建 `backend`、`frontend`、`quant`，但镜像缓存策略从 `type=registry` 切换为 `type=local + actions/cache`，目标是去掉跨境 `cache export to TCR` 的长尾耗时。
- 三个镜像必须使用独立 Buildx 本地缓存目录：
  - backend: `/tmp/.buildx-cache-backend`
  - frontend: `/tmp/.buildx-cache-frontend`
  - quant: `/tmp/.buildx-cache-quant`
- 每个镜像构建前先 restore 对应目录缓存；构建时使用 `--cache-from type=local`，输出到 `*-new` 目录，再通过目录切换覆盖旧缓存，避免 BuildKit 对同一目录读写冲突。
- 缓存 key 只绑定 Dockerfile 与依赖定义文件，不绑定业务源码全量内容：
  - backend: `backend/Dockerfile` + `backend/go.mod` + `backend/go.sum`
  - frontend: `frontend/Dockerfile` + `frontend/package-lock.json`
  - quant: `quant/Dockerfile` + `quant/requirements.txt`
- Phase 2 起，镜像构建拆成三个并行 job：`build_backend`、`build_frontend`、`build_quant`。三个 job 都依赖语言级 `build` 检查，但彼此独立执行，以总耗时接近最慢单镜像构建为目标。
- 每个并行 build job 内部各自负责：解析镜像变量、生成 `image_repo` / `image_tag` / `image_ref` 输出、登录 TCR、恢复本镜像专属本地缓存、build 并 push 最终镜像。
- `deploy` 现在必须 `needs` 三个 build job 全部成功后才会启动，并在开头打印本次发布的 backend / frontend / quant 镜像摘要；Phase 3 再补显式远端镜像存在性校验与更严格的产物闭环。
- 服务器侧部署仍按统一 `IMAGE_TAG` 从 compose 拉取 3 个镜像并启动，说明当前并行化只改变 CI 构建拓扑，不改变服务器发布方式。

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

## 旧模拟组合事实表运维链路（2026-07-01，已由 v2 替代）

- 该段记录的是 v2 重构前的历史实现，仅用于理解遗留代码；2026-07-04 起不再作为当前架构或 Admin 操作规范。
- 旧 `/portfolio-tracking` 曾读取 `portfolio_daily`、`portfolio_position`、`portfolio_trade`、`portfolio_metrics`，并通过 Admin「新口径模拟组合管理」暴露补开盘价、同步事实表、验证一致性、全局开始信号日重置等操作。
- v2 上线后，旧 `quadrant_ranking_portfolio_market_prices`、旧补价按钮、旧全局开始信号日和旧事实表同步链路仅作为历史遗留，不再作为模拟组合推进依据。
- 若后续清理 legacy 代码，应优先删除旧 Admin handler / 旧按钮 / 旧起点服务入口，再评估旧表物理 drop 的数据风险。

### 个股详情页信息架构（2026-07-01）
- `/live-trading/[symbol]` 从纵向长页面调整为「首屏决策概览 + 分域 Tab」。
- 用户任务优先级固定为：看行情、AI 判断优先；持仓管理其次。
- AI 分析是商业转化核心入口，必须在首屏概览中常驻；AI 分析历史在概览展示，新闻与公告独立成 Tab。
- PC 端 Tab 固定为：概览、走势、技术、基本面、新闻与公告、持仓 & 提醒。
- 移动端主入口固定为：概览、走势、分析、新闻、持仓；「分析」组内二级切换技术、基本面。
- Tab 配置和 URL `tab` 归一化集中在 `frontend/lib/stock-detail-tabs.js`；非法 tab 回退 `overview`。
- 本阶段不新增后端接口，继续复用个股页既有行情、技术、基础面、新闻、AI、持仓和信号配置 API。

## 模拟组合 v2 Pipeline 架构（2026-07-04）

- `/portfolio-tracking` 的长期目标数据源切换为 `Sim Portfolio v2`：`sim_portfolio_v2_daily`、`sim_portfolio_v2_positions`、`sim_portfolio_v2_trades`、`sim_portfolio_v2_metrics`。
- v2 链路由市场交易日历驱动：`market_calendars` 决定 A 股/港股某日期是否交易、上一交易日和下一交易日。
- Pipeline 阶段固定为：`calendar -> signal -> selection -> price_requirements -> entry_open -> valuation_close -> facts -> verify`。
- Admin 数据页中的模拟组合区域升级为“模拟组合 Pipeline”独立区块，展示市场状态、阶段矩阵、缺口诊断、运行日志和双市场日历驾驶舱。
- A 股和港股在 Admin 中分别展示市场日历；每个日期可查看信号、组合 A/B、开盘价、收盘价、facts 和修复建议。
- v2 开始信号日采用市场级配置：A 股和港股可独立启动，但同一市场内组合 A/B 必须同一起点；配置保存于 `sim_portfolio_v2_market_configs`。
- 缺信号修复通过 Admin 指定 `market + source_trade_date` 触发四象限重建，目标是补齐上游 `quadrant_ranking_snapshots` 后再重新运行对应日期 pipeline。
- 缺价格修复固定为三层动作：重新解析已有价格源、重拉该日缺失历史日线、人工覆盖价格。人工覆盖必须写入 `sim_portfolio_v2_price_overrides`，所有修复动作写入 `sim_portfolio_v2_price_repair_audits`。
- 价格修复只更新 price requirements / override，不直接改 verified facts；修复后必须重新运行 pipeline 才能生成正式收益。
- 旧 `quadrant_ranking_portfolio_market_prices`、旧补价按钮、旧全局开始信号日和旧事实表同步链路仅作为历史遗留，不再作为 v2 的推进依据.

## Quant Data Source Gateway（2026-07-11）

- quant 新增 `quant/data_sources/` 作为外部数据源统一入口，后续业务模块应优先通过 `DataSourceManager` 获取标准化数据，不再直接在业务代码中编排 Tencent / EastMoney / AkShare fallback。
- 第一期能力覆盖：`daily_bars`、`index_bars`；市场覆盖：A 股 `ASHARE`、港股 `HKEX`。
- 模块边界：
  - `policy.py`：按 capability + market 定义 provider 顺序，第一期仅使用代码常量，不新增 env / DB / Admin 可编辑配置。
  - `registry.py`：声明 provider 支持的 market + capability，manager 会跳过不支持的 provider。
  - `providers/`：只负责外部源调用。
  - `normalizers/`：只负责字段和单位归一。
  - `validators.py`：负责防止假成功，价格类数据必须精确匹配目标交易日。
  - `manager.py`：统一执行 fallback、trace、partial failure 返回。
- backend 长期边界保持不变：backend 只调用 quant API，不直接理解底层 provider 字段。资金星图后续应迁移为 backend proxy → quant `/api/capital-map`。

### 资金星图 quant 化链路（2026-07-13）

- `/capital-map` 前端页面仍请求 backend `/api/capital-map`，前端数据结构保持兼容。
- backend `/api/capital-map` 现在只做 quant proxy、30 秒内存缓存和 stale 降级，不再直接解析东方财富字段或计算 PE / PoC / 板块资金。
- quant 新增 `/api/capital-map` 和 `quant/capital_map/` 模块，负责资金星图 payload 构造：PE TTM 优先、成交额排序、PE 5 倍分箱 PoC、板块成交额与主力净流排序。
- quant Data Source Gateway 新增 `capital_map` capability，第一期仅支持 `ASHARE` + EastMoney provider。
