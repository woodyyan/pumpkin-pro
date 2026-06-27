# 首页内容与信息架构刷新任务

- [x] 读取 AGENTS.md 与 `.ai/context/` 长期约束。
- [x] 检查 `frontend/lib/navigation.js`，确认当前导航为「卧龙AI / 看板 / 跟踪 / 选股 / 更多」。
- [x] 检查 `frontend/pages/index.js`，识别旧 Hero、核心卖点、快速上手、功能总览和教程内容。
- [x] 新增 `frontend/data/homepage.js`，集中维护首页内容配置。
- [x] 更新 `frontend/pages/index.js`，按配置渲染新版首页结构。
- [x] 更新 `frontend/__tests__/pages/index-page-links.test.js`，覆盖新版首页内容与路由。
- [x] 运行首页专项测试。
- [x] 运行完整前端测试。
- [x] 完整测试通过，无需补充修复。
