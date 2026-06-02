# 变更记录

## 2026-06-02

- 新增历史单日快照重建入口，统一基于“交易事件 + 历史行情”生成指定 `user + scope + snapshotDate` 的组合日快照与持仓日快照。
- `RebuildDailySnapshotForUser(...)` 已切换到历史重建链路，不再复用仅适合“当天实时刷新”的 `persistDailySnapshots(...)`。
- `GetPnlCalendar(...)` 的缺失日补写继续复用 `RebuildDailySnapshotForUser(...)`，因此当前月请求链路写入的补数据也改为历史口径。
- 新增服务层测试，覆盖“历史日期重建不受未来卖出事件污染”与“pnl-calendar 自动补写缺失历史快照”两个关键场景。
