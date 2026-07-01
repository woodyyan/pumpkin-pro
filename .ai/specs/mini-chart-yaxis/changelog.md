# MiniChart Y 轴缩放变更记录

## 2026-07-01

- 新增 `frontend/lib/mini-chart-scale.js`：Y 轴缩放纯函数 `buildYAxisScale`、`buildTicks`、`roundTick`、`formatTickValue`。
- 改写 `frontend/components/MiniChart.js`：Y 轴从固定 [0, max] 改为数据 [min-pad, max+pad] 自适应，刻度显示真实数值，曲线微小波动可见。
- `MiniChart` 新增可选 props：`yAxisMode`（默认 `auto`）、`valuePrecision`（自适应）、`baselineValue`（可选基准虚线）。现有两处调用（`FactorIndexCard`、`MarketIndexCard`）零改动即生效。
- 持平 / 极小波动 / 全 0 / 跨 0 / 空 / 全非法值均有兜底。
- 新增 `frontend/lib/__tests__/mini-chart-scale.test.js`：19 个用例覆盖正常路径、持平兜底、极小波动、跨 0、zero-based、valuePrecision 自适应与显式、空/非法输入、刻度生成与格式化。
- 前端全量测试 750 项通过，`npm run build` 通过。

## 未做（独立后续项）

- `MiniChart` 硬编码 `rgba(255,255,255,...)` 文字/网格色未改，浅色主题下 Y 轴可读性差，待主题适配项处理。
- Canvas 宽度固定 320，移动端未响应式，待移动端适配项处理。
