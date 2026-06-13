# AI 分析独立页落地记录

## 2026-06-13
- 抽出前端 AI 分析公共能力：新增 `frontend/lib/ai-analysis-helpers.js`，统一封装上下文构建、`/api/search` 复用、分析执行、历史查询。
- 抽出通用展示层：新增 `frontend/components/AIAnalysisWorkspace.js` 与 `frontend/components/AIAnalysisHistorySection.js`，承接 AI 结果面板、加载态、历史卡片、质量验证摘要。
- 新建 `frontend/pages/ai/analysis.js`：支持代码/名称输入、候选下拉、`?symbol=` 预填、未登录拦截、全局历史分页展示。
- 详情页 `frontend/pages/live-trading/[symbol].js` 改为复用抽象后的 AI 控制与历史组件，保留既有入口与个股历史体验。
- 后端复用已有 `/api/search`，未新增搜索接口；新增 `/api/ai-analysis/history` 仅提供“当前用户全局分析历史分页”能力。
- `backend/store/analysis_history` 新增 `ListByUser` 分页仓储能力，并补充对应测试。
- 新增页面级前端测试 `frontend/__tests__/pages/ai-analysis-page.test.js`；后端通过 `go test ./store/analysis_history/...`。
- 现有 `frontend/__tests__/pages/ai-analysis-notification.test.js` 仍有 8 个失败用例，原因是测试仍绑定旧实现细节（直接导入 `notification`、旧状态名与内联组件定义），需后续按新抽象结构更新断言。
