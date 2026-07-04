# Changelog: 模拟组合 v2 Calendar-driven Pipeline

## 2026-07-04

- **架构决策**: 确认废弃旧模拟组合补价/快照推交易日链路，改为 Calendar-driven Portfolio Pipeline。
- **口径决策**: 采用严格模式；缺信号、缺价格或验证失败时不生成正式收益。
- **运维决策**: Admin 模拟组合区域升级为独立 Pipeline 操作台，旧“补齐开盘价/收盘价/检查日期”等按钮下线。
- **迁移决策**: 不迁移旧模拟组合历史数据；v2 从最新完整交易日开始启动。


## 2026-07-04（首轮实现）

- **后端实现**: 新增 v2 数据模型、AutoMigrate、市场交易日历服务、pipeline run/day status、信号批次、选股批次、价格需求和 v2 facts 生成。
- **接口实现**: 新增 `/api/admin/sim-portfolio-pipeline/overview|days|initialize|run`，Public `/api/portfolio-tracking/*` 切换为读取 v2 verified facts。
- **前端实现**: Admin 数据页旧模拟组合管理区块替换为“模拟组合 Pipeline”，旧补开盘价、补收盘价、同步事实表、全局开始日期按钮下线。
- **旧链路下线**: 移除旧 `/api/admin/portfolio-tracking/*` 路由；停止四象限 bulk-save 后自动调用旧 `SyncSimPortfolios`。
- **测试**: 新增 v2 后端专项测试覆盖 HKEX 休市跳过、缺信号严格阻断、价格满足后生成 verified facts；前端测试覆盖新 Admin 区块和旧按钮删除。
