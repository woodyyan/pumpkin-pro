# 个股详情页 UI 重构任务

## 已完成

- [x] 新增 `frontend/lib/stock-detail-tabs.js`，集中维护个股页 Tab 信息架构。
- [x] 新增 `frontend/lib/__tests__/stock-detail-tabs.test.js`，覆盖非法 tab、PC 顺序、移动端分组。
- [x] `/live-trading/[symbol]` 接入 URL `tab` 状态与 shallow 切换。
- [x] 首屏概览新增 AI 判断入口、技术速览、个人相关摘要。
- [x] PC 端按概览/走势/技术/基本面/新闻与公告/持仓 & 提醒分域展示。
- [x] 移动端按概览/走势/分析/新闻/持仓展示，并在分析组内提供技术/基本面二级切换。
- [x] 未登录持仓 Tab 显示登录引导。
- [x] 前端测试与构建验证。

## 后续建议

- [ ] 将各 Tab Panel 从 `frontend/pages/live-trading/[symbol].js` 拆为独立组件。
- [ ] 按 Tab 可见性延迟渲染部分重型图表。
- [ ] 若首屏并发请求压力变大，再评估后端 summary 聚合接口。
