# Tasks: 卧龙 AI 精选排行榜涨幅口径

## 已完成

- [x] 新增当前连续上榜周期起点查询。
- [x] 排行榜 API 的 `return_pct` 改为按当前连续上榜周期计算。
- [x] 修复 `quadrant_scores` upsert 不更新 `exchange` 的问题。
- [x] 扩展 `backfill-ranking-snapshots --refresh-existing`，支持刷新已有正价格历史快照。
- [x] 补充后端单元测试覆盖断档重新上榜、连续多日、缺失价格、exchange 更新与回填候选查询。

## 后续运维

- [ ] 对生产或本地目标数据库先 dry-run 历史快照价格刷新。
- [ ] 确认计划更新的快照价格后，加 `--write` 执行回填。
- [ ] 下一次 A 股象限全量计算后，确认 688/689 股票在 `quadrant_scores.exchange` 中写为 `SSE`。
