# Tasks: 因子实验室 FCFM 重设

## 已完成

- [x] Quant Phase0：新增 `capex` 字段、CapEx alias、符号归一化，以及 `--require-fcfm-inputs` 校验。
- [x] Quant Phase0 增量修复：新增 `repair_missing_fcfm_inputs` scope，按最新财务记录缺失 Revenue/OCF/CapEx 进行筛选。
- [x] Quant Phase1：新增 `free_cash_flow` / `fcf_margin` 计算，并写入 `factor_snapshots.fcf_margin`。
- [x] Quant Phase1：缺失 CapEx 时写入 `no_capex`、`no_fcf_margin` flags。
- [x] Quant Phase2：新增 `fcf_margin` metric 和 `fcf_margin_rank_score`，质量因子切换到 FCFM 组件。
- [x] Backend：质量因子说明文案改为 FCFM，raw coverage key 切换到 `fcf_margin`。
- [x] Backend Worker：支持并校验 `repair_missing_fcfm_inputs`。
- [x] Frontend Admin：修复入口、label、参数校验提示改为 FCFM 口径。
- [x] Spec：新增 `.ai/specs/factor-lab-fcfm/` 文档，明确“不做历史快照回补”。

## 已补充测试

- [x] Phase0 schema 与 CapEx 归一化测试。
- [x] Phase0 增量修复 scope 测试。
- [x] Phase1 `fcf_margin` 计算与缺 `capex` flags 测试。
- [x] Phase2 `fcf_margin_rank_score` 与 quality 因子组件测试。
- [x] Backend worker scope 与 coverage warning 测试。
- [x] Frontend admin FCFM 文案 smoke test。

## 不做

- [x] 不做历史快照回补。
- [x] 不新增新的外部财报数据源。
