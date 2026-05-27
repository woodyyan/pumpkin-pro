# Design: 因子实验室 FCFM 重设

## 1. 设计目标

1. 用可追溯的财报字段计算 FCFM，替换错误的 `operating_cf_margin` 口径。
2. 在 quant Phase0/1/2、backend factorlab、admin 页面中保持字段命名和行为一致。
3. 不做历史快照回补，只保证最新批次与未来批次正确。

## 2. 指标与边界规则

### 2.1 指标定义

1. `Operating Cash Flow (OCF)`：`经营活动产生的现金流量净额`。
2. `CapEx`：`购建固定资产、无形资产和其他长期资产支付的现金`。
3. `FCF = OCF - CapEx`。
4. `FCFM = (FCF / Revenue) * 100`。

### 2.2 CapEx 归一化

为统一不同数据源的符号差异，Phase0 将 CapEx 归一为“现金流出金额（非负数）”后入库到 `factor_financial_metrics.capex`：

1. `NULL` 保持 `NULL`。
2. 负数取 `abs(value)`。
3. 正数原样写入。

### 2.3 缺失与异常

1. `Revenue` 为 `NULL` 或 `0` 时，`fcf_margin = NULL`。
2. `operating_cash_flow` 或 `capex` 为 `NULL` 时，`free_cash_flow` / `fcf_margin = NULL`。
3. `free_cash_flow` 允许为负，`fcf_margin` 也允许为负。

## 3. 模块设计

### 3.1 Quant Phase0

职责：从财报源拉取 OCF / Revenue / CapEx，并写入结构化财务表。

实现要点：

1. 新增 CapEx alias 列表，限定为同一现金流量表财务项。
2. `factor_financial_metrics` 增量加列：`capex REAL`。
3. `--require-fcfm-inputs` 模式要求至少有一条财务记录同时具备 `revenue`、`operating_cash_flow`、`capex`。
4. 增量修复脚本新增 `repair_missing_fcfm_inputs` scope，筛选最新财务记录缺失 Revenue/OCF/CapEx 的证券重拉。

### 3.2 Quant Phase1

职责：基于最新结构化财务记录计算快照指标。

实现要点：

1. `free_cash_flow = operating_cash_flow - capex`。
2. `fcf_margin = free_cash_flow / revenue * 100`。
3. `factor_snapshots` 增量加列：`fcf_margin REAL`。
4. 数据质量 flag 新增：`no_capex`、`no_fcf_margin`。

### 3.3 Quant Phase2

职责：将快照指标转为 rank score 并计算 7 个因子分。

实现要点：

1. 原始 metric key 使用 `fcf_margin`。
2. rank score 列使用 `fcf_margin_rank_score`。
3. 质量因子公式保持不变，仅替换组件指标：
   - `quality_score = roe_rank_score * 0.35 + fcf_margin_rank_score * 0.33 + asset_to_equity_rank_score * 0.32`

### 3.4 Backend / Admin

实现要点：

1. quality 因子说明文案改为 FCFM 口径。
2. 覆盖率 raw metric key 从 `operating_cf_margin` 切换为 `fcf_margin`。
3. worker 参数校验支持 `repair_missing_fcfm_inputs`，并要求 `phase=all|phase0` 且 `phase0_mode=financials`。

### 3.5 Frontend Admin

实现要点：

1. 手动触发范围新增/替换为 `repair_missing_fcfm_inputs`。
2. 按钮、label、错误提示统一改为“自由现金流率 (FCFM)”。
3. 保留 `operating_cf_margin` label 映射仅用于兼容旧 key 展示，不再作为当前计算口径。

## 4. Schema 与兼容性

新增或确保存在的列：

1. `factor_financial_metrics.capex`
2. `factor_snapshots.fcf_margin`
3. `factor_rank_scores.fcf_margin_rank_score`

兼容策略：

1. `operating_cf_margin` 可保留为历史列，但不再参与质量因子计算。
2. 不对历史 `snapshot_date` 执行批量回算。

## 5. 验证策略

1. Phase0：测试 schema 增量、CapEx 归一化、`repair_missing_fcfm_inputs` 选股逻辑。
2. Phase1：测试 `fcf_margin` 正常计算与缺 `capex` 时的 flags。
3. Phase2：测试 `fcf_margin_rank_score` 覆盖率与 quality 因子组件替换。
4. Backend/Admin：测试 worker scope 校验、coverage warning、admin 文案。
