# 个股详情页 UI 重构设计

## 信息架构

采用「首屏决策概览 + 分域 Tab」模式。

### PC 端

固定六个 Tab：

1. 概览：实时快照、AI 入口、技术速览、个人相关摘要。
2. 走势：历史走势、区间收益、个股 vs 大盘、价量/大单异动。
3. 技术：均线、RSI、MACD、布林带、支撑位、压力位。
4. 基本面：市值、PE/PB/PEG、股息、收入、净利润、毛利率、净利率。
5. 新闻与公告：个股新闻摘要、公告与资讯列表入口。
6. 持仓 & 提醒：我的持仓、买卖/调均价表单、交易信号配置。

### 移动端

主入口压缩为五组：

1. 概览
2. 走势
3. 分析
4. 新闻
5. 持仓

「分析」组下提供二级切换：技术、基本面；新闻与公告独立成组，AI 分析历史回到概览。

## 状态与 URL

- `frontend/lib/stock-detail-tabs.js` 维护 Tab 配置、非法值归一化、移动端分组。
- URL query `tab` 是页面分类状态来源。
- 非法、空、数组等 tab 输入统一回退 `overview`。
- 切换 Tab 使用 shallow replace，避免整页刷新。

## 数据流

本阶段不改后端接口，继续复用现有页面已有请求：

- snapshot / daily-bars / overlay-daily
- fundamentals
- moving-averages / support-levels / resistance-levels
- news summary / news list
- ai-analysis / analysis-history
- portfolio / signal-configs

## 组件边界

- `StockDetailTabNav`：页面内导航组件，负责 PC 六 Tab、移动五组入口和分析二级切换。
- `stock-detail-tabs.js`：纯配置/纯函数，便于单元测试。
- 原有图表和业务面板暂不拆出，避免一次性大重构。

## 风险控制

- 不改变后端接口和字段语义。
- 不改变现有 AI 分析、持仓、信号配置提交逻辑。
- 未登录状态必须显示持仓 Tab 引导，不能出现空白页。
- 后续若页面继续增长，应再将各 Tab Panel 拆为独立组件。
