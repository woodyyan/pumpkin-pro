# AI研报变更记录

## 2026-06-27

- 新增后端 `store/aireport` 包，包含研报样例和微信服务配置的数据模型、迁移、仓储、服务与测试。
- 新增用户侧接口：
  - `GET /api/ai/reports`：公开研报缩略图列表。
  - `GET /api/ai/reports/{id}/preview`：登录后获取研报预览图。
  - `GET /api/ai/report-service-config`：公开服务配置。
- 新增 Admin 接口：
  - `GET/POST /api/admin/ai-reports`
  - `PUT/DELETE /api/admin/ai-reports/{id}`
  - `GET/PUT /api/admin/ai-report-service-config`
- 新增前端 `/ai/reports` 页面：介绍 AI研报、展示样例、登录后弹窗预览、静态套餐、企业微信 CTA、合规风险提示。
- 主站导航「卧龙AI」分组新增「AI研报」，位于「AI分析」与「AI选股」之间。
- `/admin/ai` 新增 AI研报管理和企业微信二维码/服务配置面板。
- 第一阶段套餐保持前端静态配置，不新增 pricing 表；额度核销和付费交付仅由企业微信人工管理。
- 按产品反馈优化用户页文案与套餐区：删除 Hero 内短风险提示和顶部交付时效板块；将 CTA 改为企业微信定制；将套餐调整为体验版 9.9 元、入门版 39 元、投资版 69 元、专业版 199 元，并补充自定义问题和多股票对比分析示例。
