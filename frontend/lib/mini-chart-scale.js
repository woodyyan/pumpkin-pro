/**
 * MiniChart Y 轴缩放计算（纯函数）。
 *
 * 抽离自 MiniChart.js，便于单测。只负责数值域计算，不关心画布像素。
 *
 * 设计要点：
 * - `auto` 模式（默认）：Y 轴 = [dataMin - pad, dataMax + pad]，pad = max(range*0.15, |max|*0.01)。
 *   这让波动很小的净值/指数曲线也能看出起伏，而非被 0 起点压平。
 * - `zero-based` 模式：Y 轴固定 [0, max]，保留给柱状图等必须从 0 起算的诚实可视化场景。
 * - 持平兜底：dataRange 为 0 或极小时，以数据值为中心构建 ±0.5% 虚拟范围，
 *   保证曲线不报错且视觉上是一条水平线。
 * - 刻度值显示真实数值，不缩放、不归零，由调用方决定小数位。
 */

const PADDING_RATIO = 0.15
const PADDING_MIN_RATIO = 0.01
const FLAT_THRESHOLD_RATIO = 0.001
const FLAT_HALF_SPAN_RATIO = 0.005

/**
 * 计算 Y 轴显示范围与刻度。
 *
 * @param {number[]} values 数据点（已过滤非有限值）
 * @param {object}  [options]
 * @param {'auto'|'zero-based'} [options.mode='auto']
 * @param {number} [options.tickCount=3] 区间内等分刻度数（返回 tickCount+1 个刻度，含两端）
 * @param {number} [options.valuePrecision] 刻度小数位；不传时按量级自适应
 * @returns {{ yMin: number, yMax: number, ticks: number[], valuePrecision: number } | null}
 *          values 为空或无有限值时返回 null
 */
function buildYAxisScale(values, options = {}) {
  const finite = Array.isArray(values)
    ? values.filter((v) => Number.isFinite(v))
    : []
  if (finite.length === 0) return null

  const mode = options.mode === 'zero-based' ? 'zero-based' : 'auto'
  const tickCount = Number.isFinite(options.tickCount) && options.tickCount >= 1
    ? Math.floor(options.tickCount)
    : 3

  const dataMin = Math.min(...finite)
  const dataMax = Math.max(...finite)
  const dataRange = dataMax - dataMin
  const absMax = Math.max(Math.abs(dataMin), Math.abs(dataMax), 1)

  let yMin
  let yMax

  if (mode === 'zero-based') {
    // 旧行为：0 到 max，max 至少为 1，避免除 0
    yMin = 0
    yMax = dataMax > 0 ? dataMax : 1
  } else {
    const isFlat =
      dataRange === 0 || dataRange / absMax < FLAT_THRESHOLD_RATIO
    if (isFlat) {
      // 持平或极小波动：以数据中点为中心构建虚拟范围
      // 用中点而非 dataMax，保证极小波动时中心不偏移
      // 全 0 时退化为 [0,1]
      const center = (dataMin + dataMax) / 2
      const halfSpan = Math.abs(center) * FLAT_HALF_SPAN_RATIO
      if (halfSpan === 0) {
        yMin = 0
        yMax = 1
      } else {
        yMin = center - halfSpan
        yMax = center + halfSpan
      }
    } else {
      const padding = Math.max(
        dataRange * PADDING_RATIO,
        absMax * PADDING_MIN_RATIO
      )
      yMin = dataMin - padding
      yMax = dataMax + padding
    }
  }

  // 数值精度自适应：未传时 >=100 取整，否则 2 位小数
  const valuePrecision = Number.isInteger(options.valuePrecision)
    ? options.valuePrecision
    : Math.abs(yMax) >= 100
      ? 0
      : 2

  const ticks = buildTicks(yMin, yMax, tickCount)

  return { yMin, yMax, ticks, valuePrecision }
}

/**
 * 在 [yMin, yMax] 区间内生成等距刻度值。
 * 端点固定参与，中间按 tickCount 等分。
 * 返回精确值（不取整），首尾严格等于 yMin/yMax；
 * 取整/格式化留给渲染层，确保网格线位置与文字标签一致。
 */
function buildTicks(yMin, yMax, tickCount) {
  const ticks = []
  const span = yMax - yMin
  for (let i = 0; i <= tickCount; i++) {
    ticks.push(yMin + (span * i) / tickCount)
  }
  return ticks
}

/**
 * 刻度四舍五入到指定小数位，消除浮点尾差（如 0.30000000000000004）。
 */
function roundTick(value, precision) {
  if (!Number.isFinite(value)) return 0
  const factor = Math.pow(10, precision)
  return Math.round(value * factor) / factor
}

/**
 * 把刻度值格式化为显示字符串。
 * precision=0 取整；否则按 precision 位小数。
 * 非有限值显示 '--'。
 */
function formatTickValue(value, precision) {
  if (!Number.isFinite(value)) return '--'
  const rounded = roundTick(value, precision)
  if (precision <= 0) return String(Math.round(rounded))
  return rounded.toFixed(precision)
}

module.exports = {
  buildYAxisScale,
  buildTicks,
  roundTick,
  formatTickValue,
  // 常量也导出，便于测试断言
  PADDING_RATIO,
  PADDING_MIN_RATIO,
  FLAT_THRESHOLD_RATIO,
  FLAT_HALF_SPAN_RATIO,
}
