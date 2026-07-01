import { describe, it } from 'node:test'
import assert from 'node:assert/strict'

import {
  buildYAxisScale,
  buildTicks,
  roundTick,
  formatTickValue,
  PADDING_RATIO,
  FLAT_HALF_SPAN_RATIO,
} from '../mini-chart-scale.js'

describe('buildYAxisScale — auto 模式（默认）', () => {
  it('对净值级数据自适应 min/max 并加 padding，不再从 0 起算', () => {
    // 近 20 日因子指数净值，典型 0.98 ~ 1.08
    const values = [1.0, 1.01, 0.99, 1.02, 1.05, 1.03, 1.08, 1.06]
    const scale = buildYAxisScale(values)

    assert.ok(scale)
    const { yMin, yMax } = scale
    // 不应包含 0
    assert.ok(yMin > 0, `yMin 应大于 0，实际 ${yMin}`)
    // min 略低于数据最小值 0.99
    assert.ok(yMin < 0.99, `yMin 应小于数据 min，实际 ${yMin}`)
    // max 略高于数据最大值 1.08
    assert.ok(yMax > 1.08, `yMax 应大于数据 max，实际 ${yMax}`)
    // padding 比例约为 range*0.15
    const range = 1.08 - 0.99
    const expectedPad = range * PADDING_RATIO
    assert.ok(
      Math.abs(yMin - (0.99 - expectedPad)) < 1e-9,
      `yMin padding 计算不一致：${yMin}`
    )
  })

  it('对大盘指数级数据（≥100）自适应并取整数精度', () => {
    // 上证综指近 N 日
    const values = [3150, 3180, 3170, 3220, 3240, 3210]
    const scale = buildYAxisScale(values)

    assert.ok(scale)
    assert.equal(scale.valuePrecision, 0, '≥100 时应取整数精度')
    assert.ok(scale.yMin > 3000, `yMin 应在指数区间内，实际 ${scale.yMin}`)
    assert.ok(scale.yMax > 3240, `yMax 应大于数据 max，实际 ${scale.yMax}`)
    // 刻度经格式化后应为整数字符串
    scale.ticks.forEach((t) => {
      const formatted = formatTickValue(t, scale.valuePrecision)
      assert.equal(/^\d+$/.test(formatted), true, `刻度格式化后应为整数：${formatted}`)
    })
  })

  it('刻度包含 yMin 与 yMax 两端，且按 tickCount 等分', () => {
    const values = [1.0, 1.02, 1.05, 1.03]
    const scale = buildYAxisScale(values, { tickCount: 3 })

    assert.equal(scale.ticks.length, 4, 'tickCount=3 应返回 4 个刻度（含两端）')
    assert.equal(scale.ticks[0], scale.yMin, '首刻度应等于 yMin')
    assert.equal(scale.ticks[scale.ticks.length - 1], scale.yMax, '末刻度应等于 yMax')
    // 等距
    const step = scale.ticks[1] - scale.ticks[0]
    for (let i = 1; i < scale.ticks.length; i++) {
      assert.ok(
        Math.abs(scale.ticks[i] - scale.ticks[i - 1] - step) < 1e-9,
        `刻度 ${i} 非等距`
      )
    }
  })
})

describe('buildYAxisScale — 持平与极小波动兜底', () => {
  it('全程持平时以数据值为中心构建虚拟范围，不报错', () => {
    const values = [1.0, 1.0, 1.0, 1.0]
    const scale = buildYAxisScale(values)

    const halfSpan = 1.0 * FLAT_HALF_SPAN_RATIO
    assert.ok(Math.abs(scale.yMin - (1.0 - halfSpan)) < 1e-9)
    assert.ok(Math.abs(scale.yMax - (1.0 + halfSpan)) < 1e-9)
    assert.equal(scale.valuePrecision, 2)
  })

  it('极小波动（range/absMax < 0.1%）按持平处理', () => {
    // 10000 附近波动 0.01，远小于 0.1%
    const values = [10000.0, 10000.01, 9999.99]
    const scale = buildYAxisScale(values)

    const halfSpan = 10000 * FLAT_HALF_SPAN_RATIO
    assert.ok(
      Math.abs(scale.yMin - (10000 - halfSpan)) < 1e-6,
      `极小波动应按持平兜底，yMin=${scale.yMin}`
    )
  })

  it('全 0 数据退化为 [0,1]', () => {
    const values = [0, 0, 0]
    const scale = buildYAxisScale(values)

    assert.equal(scale.yMin, 0)
    assert.equal(scale.yMax, 1)
  })
})

describe('buildYAxisScale — 跨 0 与负值', () => {
  it('数据跨 0 时 Y 轴可出现负刻度', () => {
    const values = [-1.2, -0.5, 0.3, 1.1, 0.8]
    const scale = buildYAxisScale(values)

    assert.ok(scale.yMin < -1.2, `yMin 应低于数据 min，实际 ${scale.yMin}`)
    assert.ok(scale.yMax > 1.1, `yMax 应高于数据 max，实际 ${scale.yMax}`)
    assert.ok(scale.ticks[0] < 0, '应存在负刻度')
    assert.ok(scale.ticks[scale.ticks.length - 1] > 0, '应存在正刻度')
  })
})

describe('buildYAxisScale — zero-based 模式（旧行为）', () => {
  it('固定 0 到 max，兼容旧柱状图语义', () => {
    const values = [3150, 3180, 3220]
    const scale = buildYAxisScale(values, { mode: 'zero-based' })

    assert.equal(scale.yMin, 0)
    assert.equal(scale.yMax, 3220)
    assert.equal(scale.ticks[0], 0)
    assert.equal(scale.ticks[scale.ticks.length - 1], 3220)
  })

  it('max 为 0 时退化为 [0,1]，避免除 0', () => {
    const values = [0, 0, 0]
    const scale = buildYAxisScale(values, { mode: 'zero-based' })

    assert.equal(scale.yMin, 0)
    assert.equal(scale.yMax, 1)
  })
})

describe('buildYAxisScale — valuePrecision', () => {
  it('显式传入 valuePrecision 优先于自适应', () => {
    const values = [1.0, 1.02, 1.05]
    const scale = buildYAxisScale(values, { valuePrecision: 4 })

    assert.equal(scale.valuePrecision, 4)
    // 刻度经格式化后小数位应 ≤4
    scale.ticks.forEach((t) => {
      const formatted = formatTickValue(t, scale.valuePrecision)
      const decimals = (formatted.split('.')[1] || '').length
      assert.ok(decimals <= 4, `刻度格式化后小数位应 ≤4：${formatted}`)
    })
  })

  it('未传时按量级自适应：≥100 取整，否则 2 位', () => {
    assert.equal(buildYAxisScale([1.0, 1.02]).valuePrecision, 2)
    assert.equal(buildYAxisScale([150, 160, 155]).valuePrecision, 0)
  })
})

describe('buildYAxisScale — 空数据与非法输入', () => {
  it('空数组返回 null', () => {
    assert.equal(buildYAxisScale([]), null)
  })

  it('全部非有限值返回 null', () => {
    assert.equal(buildYAxisScale([NaN, Infinity, undefined]), null)
  })

  it('混合有限/非有限值时只取有限值计算', () => {
    const scale = buildYAxisScale([1.0, NaN, 1.05, undefined, 1.02])
    assert.ok(scale)
    assert.ok(scale.yMin < 1.0)
    assert.ok(scale.yMax > 1.05)
  })
})

describe('buildTicks / roundTick / formatTickValue', () => {
  it('buildTicks 返回 tickCount+1 个等距点', () => {
    const ticks = buildTicks(0, 30, 3)
    assert.deepEqual(ticks, [0, 10, 20, 30])
  })

  it('roundTick 消除浮点尾差', () => {
    assert.equal(roundTick(0.1 + 0.2, 2), 0.3)
    assert.equal(roundTick(2.9999999, 2), 3)
    assert.equal(roundTick(3150.4, 0), 3150)
  })

  it('roundTick 对非有限值返回 0', () => {
    assert.equal(roundTick(NaN, 2), 0)
    assert.equal(roundTick(Infinity, 2), 0)
  })

  it('formatTickValue 按精度格式化', () => {
    assert.equal(formatTickValue(1.013166666, 2), '1.01')
    assert.equal(formatTickValue(3150.4, 0), '3150')
    assert.equal(formatTickValue(-0.5, 2), '-0.50')
  })

  it('formatTickValue 对非有限值显示 --', () => {
    assert.equal(formatTickValue(NaN, 2), '--')
    assert.equal(formatTickValue(Infinity, 0), '--')
  })
})
