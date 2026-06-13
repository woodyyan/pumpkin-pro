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
| `/` | `index.js` | 首页/落地页 |
| `/live-trading` | `live-trading.js` | 市场行情页（导航归属「看板」）；当前实现为 `Hero + 核心指数卡片 + 扩展指数 + 市场摘要` 的 dashboard，总览 A/H 股指数 |
| `/live-trading/[symbol]` | `live-trading/[symbol].js` | 个股详情页 (194KB)，active 仍归属「看板 / 市场行情」 |
| `/ai/analysis` | `ai/analysis.js` | AI分析占位页（一期「敬请期待」） |
| `/ai/picker` | `ai/picker.js` | AI选股占位页（一期「敬请期待」） |
| `/ai/backtest` | `ai/backtest.js` | AI回测占位页（一期「敬请期待」） |
| `/quadrant` | `quadrant.js` | 四象限独立占位页（一期「敬请期待」） |
| `/watchlist` | `watchlist.js` | 自选股占位页（一期「敬请期待」） |
| `/stock-picker` | `stock-picker.js` | 选股器 |
| `/backtest` | `backtest.js` | 回测引擎 |
| `/strategies` | `strategies.js` | 策略库 |
| `/portfolio` | `portfolio.js` | 持仓管理 |
| `/factor-lab` | `factor-lab.js` | 因子实验室 |
| `/settings` | `settings.js` | 用户设置 |
| `/admin` | `admin.js` | 管理后台 (独立布局) |
| `/changelog` | `changelog.js` | 更新日志 |
| `/share/ai-analysis-preview` | `share/ai-analysis-preview.js` | AI分析分享图预览 (独立布局) |

### 导航架构（2026-06-12）
- 主站导航不再在 `_app.js` 内硬编码多套数组，而是统一收敛到 `frontend/lib/navigation.js`。
- PC 端导航由 `components/DesktopNavMenu.js` + `components/NavDropdown.js` 渲染：hover 展开下拉，点击一级导航只负责展开/收起，不做跳转。
- 移动端导航由 `components/MobileNavMenu.js` 渲染：保留汉堡入口，菜单内部按「卧龙AI / 看板 / 跟踪 / 选股 / 更多」分组折叠，一次只展开一个分组。
- 占位页统一复用 `components/ComingSoonPage.js`，文案固定为标题 + 「敬请期待」，避免散落多个空白实现。

### 页面模式补充（2026-06-13）
- `frontend/pages/live-trading.js` 已从单行指数文本列表升级为 dashboard 页面，内部自行完成：
  - 指数标准化：`normalizeIndex(...)`
  - 页面分组：`buildMarketState(...)`
  - 市场摘要：`buildMarketInsights(...)`
  - 占位趋势图：`buildTrendSeries(...)`
- 趋势图组件继续复用 `frontend/components/MiniChart.js`，当前仅用于表达“近几次观察点的方向感”，不是专业分时图。
- `/api/live/market/overview` 尚未提供真实指数趋势数据，因此页面暂用前端推导的 7 点占位序列支撑卡片视觉；后续若后端补齐真实序列，应直接替换 `buildTrendSeries(...)` 的输入来源。
- 页面首屏核心指数固定为 6 张：上证指数、深证成指、创业板指、恒生指数、恒生中国企业指数、恒生科技指数；扩展指数预留沪深300、科创50、上证50、中证500。

### 关键组件
- `ThemeToggle.js`: 主题切换按钮（三态：浅色/深色/跟随系统）
- `NavSearchBox.js`: 导航栏股票搜索
- `DesktopNavMenu.js`: 桌面端一级导航与 hover 下拉
- `MobileNavMenu.js`: 移动端分组折叠菜单
- `NavDropdown.js`: 桌面端二级导航下拉面板
- `ComingSoonPage.js`: 占位页通用组件
- `QuadrantChart.js`: 四象限风险图表
- `RankingPanel.js` / `RankingPortfolioPanel.js`: 排行榜
- `PortfolioAttributionSection.js`: 持仓归因分析
- `AIAnalysisReportContent.js`: AI分析报告内容
- `MiniChart.js`: 轻量 canvas 迷你图，目前已复用于 admin 面板和市场行情页卡片

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
