# Tasks: 模拟组合 v2 Calendar-driven Pipeline

## Phase 1: 知识沉淀与基础模型

- [x] 新增本 spec 的 requirement/design/tasks/changelog。
- [x] 新增 v2 数据模型和 AutoMigrate。
- [x] 新增市场交易日历服务。
- [x] 新增 v2 repository 基础读写。

## Phase 2: Pipeline 后端

- [x] 新增默认 v2 组合定义初始化。
- [x] 新增 signal batch 构建。
- [x] 新增 selection batch 构建。
- [x] 新增 price requirements 构建与解析。
- [x] 新增 fact tables 生成与 verification。
- [x] 新增 overview/daily/positions/trades/metrics public 读取。

## Phase 3: Admin API 与前端

- [x] 新增 `/api/admin/sim-portfolio-pipeline/*` 接口。
- [x] Admin 四象限数据页新增“模拟组合 Pipeline”独立区块。
- [x] 删除旧模拟组合补价/同步/重算/起点按钮。

## Phase 4: 旧链路下线

- [x] Public `/api/portfolio-tracking/*` 切到 v2 读取。
- [x] 下线旧 `/api/admin/portfolio-tracking/*` 路由。
- [x] 停止 bulk-save 后自动调用旧 `SyncSimPortfolios`。
- [ ] 停止旧 realtime worker 对模拟组合 open_price 的补写依赖。

## Phase 5: 测试与验证

- [x] 后端单测覆盖 HKEX 休市跳过。
- [x] 后端单测覆盖缺信号严格阻断。
- [x] 后端单测覆盖价格需求精确日期匹配。
- [x] 后端单测覆盖 v2 public overview 只读 verified 数据。
- [x] 前端测试覆盖 Admin 新区块和旧按钮删除。
