# Changelog: 卧龙 AI 精选排行榜涨幅口径

## 2026-05-28

- **修复**: 卧龙 AI 精选排行榜的 `return_pct` 改为当前连续上榜周期以来涨幅，不再使用历史首次上榜以来涨幅。
- **修复**: `quadrant_scores` 批量写入时同步更新 `exchange`，避免科创板等股票保留旧交易所值。
- **工具**: `backfill-ranking-snapshots` 增加 `--refresh-existing`，可用于刷新已有正价格但口径错误的历史排行榜快照。
