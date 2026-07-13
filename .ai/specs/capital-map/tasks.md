# 资金星图任务

- [x] 确认路由 `/capital-map` 与导航位置。
- [x] 新增后端 `capitalmap` 服务包。
- [x] 封装东方财富个股和板块接口。
- [x] 保持 PE 选择、PoC 分箱和样本排序算法。
- [x] 新增 `/api/capital-map`。
- [x] 新增前端资金星图页面与主组件。
- [x] 引入 ECharts 并动态加载。
- [x] 适配卧龙浅色/深色主题。
- [x] 更新导航、首页功能入口和测试。
- [x] 补充后端算法/缓存测试和前端 helper/页面测试。

## Quant 迁移（2026-07-13）

- [x] quant 新增 `/api/capital-map`。
- [x] 资金星图 PE 选择、成交额排序、PoC 分箱和板块资金算法迁移到 quant。
- [x] Data Source Gateway 新增 `capital_map` capability。
- [x] EastMoney provider 增加 A 股资金星图股票和板块公开接口。
- [x] backend `/api/capital-map` 改为 quant proxy。
- [x] backend 保留 30 秒缓存与 stale 降级。

## 后续可选

- [ ] 登录后高亮自选股。
- [ ] 点击散点后跳转个股详情或展示移动端底部详情条。
- [ ] 增加后台 worker + SQLite 快照，降低实时外部接口压力。
- [ ] 增加 admin 运维面板，展示最近成功刷新、失败原因和关键字段覆盖率。
