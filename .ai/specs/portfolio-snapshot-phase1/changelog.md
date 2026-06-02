# 变更记录

## 2026-06-02

- 新增历史单日快照重建入口，统一基于“交易事件 + 历史行情”生成指定 `user + scope + snapshotDate` 的组合日快照与持仓日快照。
- `RebuildDailySnapshotForUser(...)` 已切换到历史重建链路，不再复用仅适合“当天实时刷新”的 `persistDailySnapshots(...)`。
- `GetPnlCalendar(...)` 的缺失日补写继续复用 `RebuildDailySnapshotForUser(...)`，因此当前月请求链路写入的补数据也改为历史口径。
- 新增服务层测试，覆盖“历史日期重建不受未来卖出事件污染”与“pnl-calendar 自动补写缺失历史快照”两个关键场景。
- 新增 `backend/store/portfolio/worker.go`，提供按市场拆分的日快照定时 worker：A 股固定北京时间 16:00 后触发、港股固定北京时间 17:00 后触发，并统一调用 `RunDailyMarketSnapshot(...)`。
- 新增 `backend/cmd/rebuild-portfolio-daily-snapshots/main.go`，支持按市场或按用户手动触发历史单日快照重建，CLI 复用同一服务口径。
- 新增 `backend/store/portfolio/worker_test.go` 与 `backend/cmd/rebuild-portfolio-daily-snapshots/main_test.go`，覆盖 worker 执行与 CLI 参数规范化行为。
