# Requirement: 因子实验室 FCFM 重设（替换 operating_cf_margin）

## 背景

因子实验室「质量」因子历史上包含 FCFM 指标，但由于无法直接拿到 FCFM，曾临时用 `operating_cf_margin`（经营活动现金流量净额 / 营业收入）替代。这会改变指标语义与投资口径，导致「质量」因子与产品承诺不一致。

本次需求：按明确公式重新设计并落地 FCFM 计算链路，并在量化计算、后端覆盖率/管理后台、前端 admin 文案中完成同步。

## 目标

1. 恢复 FCFM 的正确口径：
   - `FCFM = (Free Cash Flow / Revenue) * 100%`
   - `Free Cash Flow (FCF) = Operating Cash Flow - CapEx`
2. CapEx 口径固定为现金流量表字段：`购建固定资产、无形资产和其他长期资产支付的现金`。
3. 保证“最新数据”和“未来数据”计算正确；不做历史快照回补。
4. 「质量」因子计算公式保持不变：
   - `quality_score = ROE_rank_score * 0.35 + FCFM_rank_score * 0.33 + asset_to_equity_rank_score * 0.32`
5. 管理后台可观测并可修复：覆盖率统计与修复入口支持 FCFM 所需字段。

## 范围

### In Scope

1. Quant Phase0：新增/完善财务结构化字段以支持 FCFM，落库到 `factor_financial_metrics.capex`。
2. Quant Phase1：从 Phase0 结构化表读取并计算 `factor_snapshots.fcf_margin`。
3. Quant Phase2：将 `fcf_margin` 纳入 rank score，并用于 `quality_score` 的组合计算；rank 列为 `factor_rank_scores.fcf_margin_rank_score`。
4. Backend：
   - 覆盖率统计与 warning 口径切换到 `fcf_margin`。
   - Admin 触发流水线/修复入口支持 `repair_missing_fcfm_inputs`。
5. Frontend admin：
   - 文案从“经营现金流率”迁移为“自由现金流率 (FCFM)”。
   - 修复按钮和参数校验与后端 scope 对齐。

### Out of Scope

1. 历史快照回补/重算所有历史交易日。
2. 引入新的外部数据源。

## 业务口径与计算规则

### 指标定义

1. `Operating Cash Flow (OCF)`：现金流量表字段 `经营活动产生的现金流量净额`。
2. `CapEx`：现金流量表字段 `购建固定资产、无形资产和其他长期资产支付的现金`。
3. `FCF = OCF - CapEx`
4. `FCFM = (FCF / Revenue) * 100%`

### 交易日与数据日期

本需求不改变交易日口径与 `source_trade_date` 规则，但新增/调整的离线产出仍应遵守：用户口径日期使用 `source_trade_date`，`computed_at` 仅用于运维排障。

## 验收标准

1. 任意一只股票在最新一期可得财务数据齐全时，`factor_snapshots.fcf_margin` 可计算且为百分比值。
2. `quality_score` 的组成从 `operating_cf_margin_rank_score` 改为 `fcf_margin_rank_score`，权重不变（0.35/0.33/0.32）。
3. Admin 覆盖率与 warning 不再把 `operating_cf_margin` 当作质量因子组件指标；应展示 `fcf_margin` 覆盖率。
4. Admin 具备 `repair_missing_fcfm_inputs` 修复入口，并有明确的参数校验与错误提示。
5. 不做历史快照回补。

## 依赖与前置

1. Phase0 能从财报数据中拿到 CapEx 对应字段，并支持 alias 适配。
2. 若不同数据源对该字段缺失或命名不稳定，需在 Phase0 增强 require 模式与覆盖率告警，避免“写入成功但字段为空”的假成功（参见 BP-002）。
