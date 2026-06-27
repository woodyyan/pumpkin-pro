# AI研报设计

## 信息架构

主站导航「卧龙AI」分组顺序：

1. AI分析 `/ai/analysis`
2. AI研报 `/ai/reports`
3. AI选股 `/ai/picker`
4. AI回测 `/ai/backtest`

## 页面设计

`/ai/reports` 为公开页面，承担介绍、样例展示和企业微信转化，不承担站内交易功能。

页面结构：

1. Hero：说明 AI研报是面向个股的深度投资研究报告。
2. 亮点卡片：覆盖市场、专业深度研报、投资建议、多维分析、目标位/止损位、财报与事件解读。
3. 研报样例：卡片展示缩略图、股票名称、代码、市场、`source_trade_date`。
4. 弹窗预览：登录后加载预览图；未登录点击时唤起登录弹窗。
5. 套餐价格：前端静态写死 4 档套餐（体验版、入门版、投资版、专业版）。
6. 企业微信 CTA：读取后台配置的企业微信号与二维码。
7. 合规风险：页面显著展示完整免责声明，弹窗展示短风险提示。

## 权限设计

- `/api/ai/reports`：可匿名访问，只返回缩略图 URL 和基础元数据。
- `/api/ai/reports/{id}/preview`：必须登录，只返回预览图 URL，不返回原图。
- 用户侧永不返回 `image_original_key` 或原图 URL。
- Admin 接口必须超级管理员登录。

## 数据模型

### ai_research_reports

核心业务字段：

- `stock_name`
- `symbol`
- `exchange`
- `source_trade_date`
- `image_original_key`
- `image_preview_key`
- `image_thumbnail_key`

系统字段：

- `id`
- `created_at`
- `updated_at`

### ai_report_service_config

用于后台配置企业微信服务入口：

- `wechat_id`
- `wechat_qr_image_key`
- `delivery_time_text`
- `risk_disclaimer`

## COS Key / URL 规则

第一阶段 admin 表单管理 COS 对象 Key 或完整 URL，不做站内直传。后端根据 `COS_BUCKET` 与 `COS_REGION` 把相对 Key 转为公网 COS URL；如果字段本身是完整 URL 或站内路径，则原样返回。

推荐路径：

- 原图：`ai-reports/original/{year}/{report_id}.png`
- 预览图：`ai-reports/preview/{year}/{report_id}.webp`
- 缩略图：`ai-reports/thumb/{year}/{report_id}.webp`
- 企业微信二维码：`ai-reports/service/wechat-qr.png`

## 套餐设计

第一阶段套餐仍写在前端静态配置中：

- 体验版：9.9 元，1 份标准 PDF 报告，单份 9.9 元。
- 入门版：39 元，5 份标准 PDF 报告，单份 7.8 元。
- 投资版：69 元，10 份标准 PDF 报告，单份 6.9 元，增加投资周期定制、持仓情况分析、自定义分析问题。
- 专业版：199 元，50 份标准 PDF 报告，单份 3.98 元，包含投资版全部服务，并增加优先生成、多股票对比分析、新功能优先体验。

页面需在套餐区明确提示：每份报告只包含一只股票的分析结果；每个套餐卡片需用小标签展示单份价格，帮助用户判断性价比。

自定义问题示例：我的成本 42 元，要不要止损？未来一年最大的风险是什么？为什么该股票近一个月一直跌？现在该股票还能加仓吗？

多股票对比分析示例：帮我比较小米 vs 比亚迪 vs 宁德时代的股票。

## 取舍

第一阶段刻意不做订单、支付、额度、购买状态、原图下载和自动研报生成，避免过早复杂化。用户转化由企业微信人工闭环，站内只做展示和预览权限控制。
