# Changelog: 因子实验室 FCFM 重设

## Unreleased

### Changed

- 因子实验室「质量」因子组件从 `operating_cf_margin` 切换为 `FCFM` 口径，质量公式保持不变：`ROE_rank_score * 0.35 + FCFM_rank_score * 0.33 + asset_to_equity_rank_score * 0.32`。
- 后端质量因子说明文案改为“ROE、自由现金流率（FCFM）与资产权益比排名分加权后的质量得分”。
- Admin 修复 scope 从旧的经营现金流语义切换为 `repair_missing_fcfm_inputs`。

### Added

- 东方财富 direct API 兼容当前可用字段与报表：
  - 利润表优先使用旧 `RPT_LICO_FN_CPD`，为空时兜底 `RPT_DMSK_FN_INCOME`。
  - 英文字段兼容 `SECURITY_CODE` / `SECUCODE`、`TOTAL_OPERATE_INCOME`、`PARENT_NETPROFIT`、`TOTAL_ASSETS`、`TOTAL_EQUITY`、`NETCASH_OPERATE`、`CONSTRUCT_LONG_ASSET`。
- 新增 FCFM 计算链路：
  - `FCF = 经营活动现金流量净额 - CapEx`
  - `FCFM = FCF / 营业收入 * 100%`
- `factor_financial_metrics.capex`：CapEx 结构化字段，口径固定为 `购建固定资产、无形资产和其他长期资产支付的现金`。
- `factor_snapshots.fcf_margin`：最新快照中的自由现金流率指标。
- `factor_rank_scores.fcf_margin_rank_score`：FCFM 对应的排名分字段。
- Admin 原始指标覆盖率与 warning 改为基于 `fcf_margin` 展示。

### Deprecated

- `operating_cf_margin` 不再作为质量因子组件指标使用；若历史列仍存在，仅用于兼容旧数据，不代表当前口径。

### Notes

- 不做历史快照回补：仅保证最新快照和未来快照使用新口径正确计算。
