import { describe, it } from 'node:test'
import assert from 'node:assert/strict'

import {
  buildDefaultSignalConfig,
  normalizeSignalConfig,
  hasSignalConfigChanged,
  buildSignalConfigPayload,
  buildSignalConfigMeta,
  buildPositionAwareSummary,
  describeSignalGate,
  buildPositionSnapshotRows,
} from '../signal-config-ui.js'

// ──────────────────────────────────────────────
// 持仓感知门控：前端配置与展示
// ──────────────────────────────────────────────

describe('持仓感知配置默认值', () => {
  it('持仓感知默认开启（只影响推送，无交易决策风险）', () => {
    const config = buildDefaultSignalConfig('000001.SZ', [])
    assert.equal(config.position_aware_enabled, true)
  })

  it('止损与移动止盈默认关闭（0）', () => {
    const config = buildDefaultSignalConfig('000001.SZ', [])
    assert.equal(config.stop_loss_pct, 0)
    assert.equal(config.trailing_stop_pct, 0)
  })

  it('老数据缺少 position_aware_enabled 时应视为开启', () => {
    const config = normalizeSignalConfig({ symbol: '000001.SZ', strategy_id: 's1' }, '000001.SZ', [])
    assert.equal(config.position_aware_enabled, true)
  })

  it('显式 false 应被保留', () => {
    const config = normalizeSignalConfig(
      { symbol: '000001.SZ', position_aware_enabled: false },
      '000001.SZ',
      []
    )
    assert.equal(config.position_aware_enabled, false)
  })

  it('负数与非法风控值归一为 0（关闭）', () => {
    const config = normalizeSignalConfig(
      { symbol: '000001.SZ', stop_loss_pct: -5, max_position_pct: 'abc', max_add_times: -1 },
      '000001.SZ',
      []
    )
    assert.equal(config.stop_loss_pct, 0)
    assert.equal(config.max_position_pct, 0)
    assert.equal(config.max_add_times, 0)
  })
})

describe('风控字段脏检查', () => {
  const base = normalizeSignalConfig({ symbol: 'X', strategy_id: 's1' }, 'X', [])

  it('修改止损应判定为有变更', () => {
    const draft = { ...base, stop_loss_pct: 8 }
    assert.equal(hasSignalConfigChanged(base, draft), true)
  })

  it('修改持仓感知开关应判定为有变更', () => {
    const draft = { ...base, position_aware_enabled: false }
    assert.equal(hasSignalConfigChanged(base, draft), true)
  })

  it('未修改时不应判定为有变更', () => {
    assert.equal(hasSignalConfigChanged(base, { ...base }), false)
  })
})

describe('buildSignalConfigPayload 风控字段', () => {
  it('始终显式下发风控字段，保证用户可把规则调回 0 关闭', () => {
    const payload = buildSignalConfigPayload({
      strategy_id: 's1',
      is_enabled: true,
      stop_loss_pct: 0,
      max_position_pct: 0,
      position_aware_enabled: true,
    })
    assert.equal(payload.stop_loss_pct, 0)
    assert.equal(payload.max_position_pct, 0)
    assert.equal(payload.position_aware_enabled, true)
  })

  it('风控值应原样传递', () => {
    const payload = buildSignalConfigPayload({
      strategy_id: 's1',
      stop_loss_pct: 8,
      max_position_pct: 20,
      max_add_times: 3,
      trailing_stop_pct: 5,
    })
    assert.equal(payload.stop_loss_pct, 8)
    assert.equal(payload.max_position_pct, 20)
    assert.equal(payload.max_add_times, 3)
    assert.equal(payload.trailing_stop_pct, 5)
  })
})

describe('buildPositionAwareSummary', () => {
  it('关闭时返回已关闭', () => {
    assert.equal(buildPositionAwareSummary({ position_aware_enabled: false }), '已关闭')
  })

  it('开启但无额外限制时返回已开启', () => {
    assert.equal(buildPositionAwareSummary({ position_aware_enabled: true }), '已开启')
  })

  it('罗列已启用的风控项', () => {
    const summary = buildPositionAwareSummary({
      position_aware_enabled: true,
      max_position_pct: 20,
      stop_loss_pct: 8,
    })
    assert.ok(summary.includes('单票上限 20%'))
    assert.ok(summary.includes('止损 8%'))
  })
})

describe('buildSignalConfigMeta 含持仓感知', () => {
  it('meta 中应包含持仓感知条目', () => {
    const meta = buildSignalConfigMeta({
      config: { strategy_id: 'macd', is_enabled: true, position_aware_enabled: true },
      strategyMap: { macd: { name: 'MACD' } },
    })
    const item = meta.find((entry) => entry.label === '持仓感知')
    assert.ok(item, 'meta 应含持仓感知')
    assert.equal(item.value, '已开启')
  })
})

describe('describeSignalGate', () => {
  it('老数据无门控字段时按已推送处理', () => {
    const got = describeSignalGate({ side: 'BUY' })
    assert.equal(got.decision, 'pass')
    assert.equal(got.isSuppressed, false)
  })

  it('被拦截的信号应弱化但可解释', () => {
    const got = describeSignalGate({
      gate_decision: 'suppressed',
      suppressed_reason: 'GATE_SELL_NO_POSITION',
      suppressed_message: '你当前未持有该股，卖出提示已归档但未推送。',
    })
    assert.equal(got.isSuppressed, true)
    assert.equal(got.tone, 'muted', '被拦截应弱化展示而非隐藏')
    assert.ok(got.message.length > 0, '必须给出可读原因')
    assert.equal(got.label, '已归档（未推送）')
  })

  it('风控强制信号应醒目并标明方向被改写', () => {
    const got = describeSignalGate({
      gate_decision: 'overridden',
      raw_side: 'BUY',
      final_side: 'SELL',
      semantic_label: '止损离场',
    })
    assert.equal(got.tone, 'warning')
    assert.equal(got.isOverridden, true)
    assert.equal(got.isSideRewritten, true, '策略说买、风控改卖必须明示')
    assert.equal(got.semanticLabel, '止损离场')
  })

  it('方向未被改写时 isSideRewritten 为 false', () => {
    const got = describeSignalGate({ gate_decision: 'pass', raw_side: 'BUY', final_side: 'BUY' })
    assert.equal(got.isSideRewritten, false)
  })

  it('空输入返回 null', () => {
    assert.equal(describeSignalGate(null), null)
  })
})

describe('buildPositionSnapshotRows', () => {
  it('未录持仓与空仓必须区分表述', () => {
    const unknown = buildPositionSnapshotRows({ data_status: 'unknown' })
    assert.equal(unknown[0].value, '未录入')

    const empty = buildPositionSnapshotRows({ data_status: 'known', shares: 0 })
    assert.equal(empty[0].value, '当前空仓')
  })

  it('有持仓时展示数量/成本/浮盈亏/仓位占比', () => {
    const rows = buildPositionSnapshotRows({
      data_status: 'known',
      shares: 1000,
      avg_cost_price: 100,
      unrealized_pnl_pct: 10,
      position_weight_pct: 11,
    })
    const labels = rows.map((r) => r.label)
    assert.ok(labels.includes('持仓数量'))
    assert.ok(labels.includes('持仓成本'))
    assert.ok(labels.includes('浮动盈亏'))
    assert.ok(labels.includes('仓位占比'))
  })

  it('盈利用 up 色、亏损用 down 色（A股口径由渲染层映射红涨绿跌）', () => {
    const win = buildPositionSnapshotRows({
      data_status: 'known', shares: 100, avg_cost_price: 10, unrealized_pnl_pct: 5,
    })
    assert.equal(win.find((r) => r.label === '浮动盈亏').tone, 'up')

    const lose = buildPositionSnapshotRows({
      data_status: 'known', shares: 100, avg_cost_price: 10, unrealized_pnl_pct: -5,
    })
    assert.equal(lose.find((r) => r.label === '浮动盈亏').tone, 'down')
  })

  it('亏损时浮盈亏带负号且不隐藏（不做处置效应偏向）', () => {
    const rows = buildPositionSnapshotRows({
      data_status: 'known', shares: 100, avg_cost_price: 100, unrealized_pnl_pct: -50,
    })
    const pnl = rows.find((r) => r.label === '浮动盈亏')
    assert.equal(pnl.value, '-50.00%')
  })

  it('stale 持仓应附加提示', () => {
    const rows = buildPositionSnapshotRows({
      data_status: 'stale', shares: 100, avg_cost_price: 10,
    })
    assert.ok(rows.some((r) => r.value === '较久未更新'))
  })

  it('空输入返回空数组', () => {
    assert.deepEqual(buildPositionSnapshotRows(null), [])
  })
})
