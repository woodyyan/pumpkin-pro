# MiniChart Y 轴自适应缩放 — 设计方案

## 方案选型

对比三个候选：

| 方案 | 做法 | 优点 | 缺点 |
|---|---|---|---|
| **A. 自适应 min/max + padding（采纳）** | Y 轴 = [dataMin − pad, dataMax + pad] | 保留绝对值语义；符合金融图表惯例（Bloomberg、东方财富、同花顺）；改动最小 | 极小波动会被视觉放大，需靠刻度值让用户感知量级 |
| B. 起点归零（用户原方案） | 所有点减起点值 | 跨指数可比 | Y 轴变「相对变化」语义，与卡片上方绝对值/涨跌幅割裂；起点敏感 |
| C. 百分比归一化 | 标准化为指数化走势 | 跨指数可比性最强 | 完全丢失绝对值，与已有涨跌幅文本重复 |

**结论**：选 A。卡片头部数字已承担「绝对量级 + 涨跌幅」信息，曲线只需辅助呈现形状与拐点；A 是改动最小、语义最一致、行业最标准的做法。

## 核心算法

```
dataMin, dataMax = min(values), max(values)
dataRange = dataMax - dataMin
absMax = max(|dataMin|, |dataMax|, 1)

if mode == 'zero-based':
    yMin, yMax = 0, max(dataMax, 1)
else:  // auto
    isFlat = dataRange == 0 || dataRange / absMax < 0.001
    if isFlat:
        center = (dataMin + dataMax) / 2   // 用中点而非 dataMax，避免极小波动时中心偏移
        halfSpan = |center| * 0.005
        if halfSpan == 0: yMin, yMax = 0, 1   // 全 0 兜底
        else: yMin, yMax = center ∓ halfSpan
    else:
        padding = max(dataRange * 0.15, absMax * 0.01)
        yMin, yMax = dataMin - padding, dataMax + padding

valuePrecision = 显式传入 ? 传入 : (|yMax| >= 100 ? 0 : 2)
ticks = [yMin, yMin + span/n, ..., yMax]  // 精确值，不取整；取整交给渲染层
```

### 关键常量

| 常量 | 值 | 语义 |
|---|---|---|
| `PADDING_RATIO` | 0.15 | 正常波动下上下留白比例 |
| `PADDING_MIN_RATIO` | 0.01 | padding 下限，防 range=0 退化 |
| `FLAT_THRESHOLD_RATIO` | 0.001 | range/absMax < 0.1% 视为持平 |
| `FLAT_HALF_SPAN_RATIO` | 0.005 | 持平时虚拟范围 ±0.5% |

### 刻度与渲染对齐

- `buildTicks` 返回**精确值**，首尾严格等于 yMin/yMax。
- 渲染层按索引等分画布绘制网格线，并用 `formatTickValue(tick, precision)` 格式化文字。
- 这样网格线位置与文字标签严格一致，避免取整导致错位。

## 接口契约

`MiniChart` 新增三个可选 props，默认值保证现有调用零改动：

| Prop | 类型 | 默认 | 语义 |
|---|---|---|---|
| `yAxisMode` | `'auto' \| 'zero-based'` | `'auto'` | auto=自适应；zero-based=旧行为（0 到 max），保留给未来柱状图 |
| `valuePrecision` | `number` | 自适应 | Y 轴刻度小数位；不传时 ≥100 取整、否则 2 位 |
| `baselineValue` | `number` | `undefined` | 可选基准线值，在图上画虚线参考 |

现有两处调用（`FactorIndexCard`、`MarketIndexCard`）**无需改动 props**，默认 auto 即生效。

## 模块划分

```
lib/mini-chart-scale.js   ← 纯函数：buildYAxisScale / buildTicks / roundTick / formatTickValue
components/MiniChart.js    ← 渲染层：调用纯函数，按返回值画图
lib/__tests__/mini-chart-scale.test.js  ← 纯函数单测
```

## 边界情况

| 场景 | 处理 |
|---|---|
| 数据点 < 2 | 调用方已拦截，不渲染 |
| 全程持平 | 以中点为中心构建 ±0.5% 虚拟范围 |
| 极小波动（<0.1%） | 同持平 |
| 全 0 | 退化为 [0,1] |
| 跨 0 数据 | Y 轴可出现负刻度，按真实 min/max |
| 空/全非法值 | 返回 null，渲染层兜底 [0,1] |

## 风险

| 风险 | 缓解 |
|---|---|
| 自适应放大波动，用户误以为「涨很多」 | Y 轴刻度显示真实数值；卡片头部涨跌幅文本为权威信息 |
| 跨卡片对比失真 | 卡片头部涨跌幅是跨卡对比权威依据；sparkline 不适合精确跨卡对比 |
| Canvas 难单测像素 | 缩放逻辑抽纯函数单测数值正确性 |
