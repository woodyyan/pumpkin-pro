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
| `/live-trading` | `live-trading.js` | 行情看板概览 |
| `/live-trading/[symbol]` | `live-trading/[symbol].js` | 个股详情页 (194KB) |
| `/stock-picker` | `stock-picker.js` | 选股器 |
| `/backtest` | `backtest.js` | 回测引擎 |
| `/strategies` | `strategies.js` | 策略库 |
| `/portfolio` | `portfolio.js` | 持仓管理 |
| `/factor-lab` | `factor-lab.js` | 因子实验室 |
| `/settings` | `settings.js` | 用户设置 |
| `/admin` | `admin.js` | 管理后台 (独立布局) |
| `/changelog` | `changelog.js` | 更新日志 |
| `/share/ai-analysis-preview` | `share/ai-analysis-preview.js` | AI分析分享图预览 (独立布局) |

### 关键组件
- `ThemeToggle.js`: 主题切换按钮（三态：浅色/深色/跟随系统）
- `NavSearchBox.js`: 导航栏股票搜索
- `QuadrantChart.js`: 四象限风险图表
- `RankingPanel.js` / `RankingPortfolioPanel.js`: 排行榜
- `PortfolioAttributionSection.js`: 持仓归因分析
- `AIAnalysisReportContent.js`: AI分析报告内容
