# Changelog: 模拟组合 v2 Calendar-driven Pipeline

## 2026-07-06（缺陷修复：A 股组合科创板排除失效）

- **问题**: v2 重构首个提交起，`defaultSimPortfolioV2Definitions()` 中 A 股 `模拟组合A`（`spv2_ashare_a`）、`模拟组合B`（`spv2_ashare_b`）遗漏了 `ExcludedBoards: [aShareBoardStar]`，导致科创板（688/689）个股未被排除，选股口径与旧版 `portfolio_service.go` 不一致。港股两个组合（`spv2_hkex_a`/`spv2_hkex_b`）本身不涉及科创板，无需处理，行为不受影响。
- **修复**: 在 `sim_portfolio_v2_repository.go` 为 A 股组合 A/B 显式补上 `ExcludedBoards: mustMarshal([]string{aShareBoardStar})`；`EnsureSimPortfolioV2Definitions` 的 upsert 已覆盖 `excluded_boards` 列，服务下次触发定义初始化即可生效，无需额外迁移脚本。
- **测试**: 新增 `TestSimPortfolioV2DefinitionsExcludeStarBoardForAShareOnly`（校验默认定义本身的 `ExcludedBoards` 值）与 `TestSimPortfolioV2SelectionExcludesStarBoardForAShare`（端到端校验混合科创板/主板信号时，A 股组合 A 选股结果不含科创板代码）。
- **历史数据**: 本次修复仅影响代码默认定义与后续新交易日的选股口径；已生成的历史 `sim_portfolio_v2_daily/positions/trades` 是否需要针对历史交易日重跑 pipeline 回溯修正，由业务方另行处理，不在本次修复范围内。
- **参见**: `.ai/memory/bug-patterns.md` BP-019。

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

## 2026-07-04（市场日历驾驶舱）

- **Admin 可观测性**: 新增 A 股/港股双市场日历驾驶舱，按月查看每个市场每天的模拟组合 v2 状态。
- **日期详情**: 支持查看某市场某日信号快照、组合 A/B 成分股数量、建仓开盘价完整度、估值收盘价完整度、facts 状态和修复建议。
- **市场独立起点**: 新增市场级开始信号日 preview/apply 基础接口，A 股和港股可独立启动；同一市场内组合 A/B 共用起点。
- **配置持久化**: 新增 `sim_portfolio_v2_market_configs` 保存市场级起点、发布 job 和最新发布估值日。
- **测试**: 补充后端测试覆盖双市场日历、缺价详情、未来日期/休市日起点拒绝；前端测试覆盖日历驾驶舱文案和 API 映射。

## 2026-07-04（缺口修复闭环）

- **缺信号修复**: 新增 Admin 指定 `market + source_trade_date` 的四象限重建入口，用于修复模拟组合 v2 日历中的 `missing_signal`；触发后仍需重新运行对应日期 pipeline 才会生成正式 facts。
- **价格三层修复**: 新增 `prices/resolve`、`prices/backfill-daily-bars`、`prices/override` 三类 Admin 动作：先重新解析已有价格源，再重拉该日缺失历史日线，最后才允许人工覆盖价格。
- **审计约束**: 人工覆盖价格必须填写 reason/evidence 并 `confirm=true`；覆盖记录写入 `sim_portfolio_v2_price_overrides`，所有修复动作写入 `sim_portfolio_v2_price_repair_audits`。
- **UI 实现**: Admin 日期详情中的修复建议从静态标签升级为可执行按钮，并提供人工覆盖价格表单；修复价格不会直接改 facts，需重新运行 pipeline。
- **测试**: 后端覆盖重新解析、历史日线重拉、人工覆盖审计与 handler；前端静态测试覆盖新修复入口。
