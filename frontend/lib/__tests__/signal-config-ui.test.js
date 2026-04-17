import { describe, it } from 'node:test'
import assert from 'node:assert/strict'

import {
  buildDefaultSignalConfig,
  normalizeSignalConfig,
  hasSignalConfigChanged,
  canEnableSignal,
  buildSignalConfigPayload,
  mergeServerSignalConfig,
} from '../signal-config-ui.js'

const strategies = [
  { id: 'macd', name: 'MACD' },
  { id: 'trend', name: '趋势' },
]

function makeConfig(overrides = {}) {
  return {
    symbol: '600036.SH',
    strategy_id: 'macd',
    is_enabled: false,
    cooldown_seconds: 3600,
    eval_interval_seconds: 3600,
    thresholds: {},
    ...overrides,
  }
}

describe('buildDefaultSignalConfig()', () => {
  it('uses first strategy as default strategy_id', () => {
    const result = buildDefaultSignalConfig('600036.SH', strategies)
    assert.equal(result.symbol, '600036.SH')
    assert.equal(result.strategy_id, 'macd')
    assert.equal(result.is_enabled, false)
    assert.equal(result.eval_interval_seconds, 3600)
  })

  it('falls back safely when no strategy exists', () => {
    const result = buildDefaultSignalConfig('00700.HK', [])
    assert.equal(result.strategy_id, '')
    assert.deepEqual(result.thresholds, {})
  })
})

describe('normalizeSignalConfig()', () => {
  it('returns defaults when input is null', () => {
    const result = normalizeSignalConfig(null, '600036.SH', strategies)
    assert.equal(result.strategy_id, 'macd')
    assert.equal(result.is_enabled, false)
  })

  it('normalizes missing values from server payload', () => {
    const result = normalizeSignalConfig({ symbol: '600036.SH', is_enabled: 1 }, '600036.SH', strategies)
    assert.equal(result.strategy_id, '')
    assert.equal(result.is_enabled, true)
    assert.equal(result.cooldown_seconds, 3600)
    assert.equal(result.eval_interval_seconds, 3600)
    assert.deepEqual(result.thresholds, {})
  })
})

describe('hasSignalConfigChanged()', () => {
  it('returns false when configs are identical', () => {
    const base = makeConfig()
    assert.equal(hasSignalConfigChanged(base, { ...base }), false)
  })

  it('detects strategy changes', () => {
    assert.equal(hasSignalConfigChanged(makeConfig(), makeConfig({ strategy_id: 'trend' })), true)
  })

  it('detects eval interval changes', () => {
    assert.equal(hasSignalConfigChanged(makeConfig(), makeConfig({ eval_interval_seconds: 900 })), true)
  })

  it('detects enabled state changes', () => {
    assert.equal(hasSignalConfigChanged(makeConfig(), makeConfig({ is_enabled: true })), true)
  })

  it('treats threshold object key order as unchanged', () => {
    const server = makeConfig({ thresholds: { b: 2, a: 1 } })
    const draft = makeConfig({ thresholds: { a: 1, b: 2 } })
    assert.equal(hasSignalConfigChanged(server, draft), false)
  })
})

describe('canEnableSignal()', () => {
  it('allows enabling when strategy exists', () => {
    assert.deepEqual(canEnableSignal(makeConfig()), { ok: true, reason: '' })
  })

  it('blocks enabling when strategy is missing', () => {
    assert.deepEqual(canEnableSignal(makeConfig({ strategy_id: '' })), {
      ok: false,
      reason: '请先选择策略，再开启信号',
    })
  })
})

describe('buildSignalConfigPayload()', () => {
  it('builds payload from config and keeps other fields', () => {
    const result = buildSignalConfigPayload(makeConfig({ eval_interval_seconds: 900, thresholds: { score: 0.8 } }), true)
    assert.deepEqual(result, {
      strategy_id: 'macd',
      is_enabled: true,
      cooldown_seconds: 3600,
      eval_interval_seconds: 900,
      thresholds: { score: 0.8 },
    })
  })

  it('falls back to defaults when numbers are invalid', () => {
    const result = buildSignalConfigPayload(makeConfig({ cooldown_seconds: 0, eval_interval_seconds: NaN }))
    assert.equal(result.cooldown_seconds, 3600)
    assert.equal(result.eval_interval_seconds, 3600)
  })
})

describe('mergeServerSignalConfig()', () => {
  it('uses server config when clean and not toggling', () => {
    const server = makeConfig({ strategy_id: 'trend' })
    const draft = makeConfig({ strategy_id: 'macd' })
    assert.deepEqual(mergeServerSignalConfig({ serverConfig: server, draftConfig: draft, isDirty: false, isToggling: false }), server)
  })

  it('keeps local draft while dirty', () => {
    const server = makeConfig({ strategy_id: 'trend' })
    const draft = makeConfig({ strategy_id: 'macd' })
    assert.deepEqual(mergeServerSignalConfig({ serverConfig: server, draftConfig: draft, isDirty: true, isToggling: false }), draft)
  })

  it('keeps optimistic draft while toggling', () => {
    const server = makeConfig({ is_enabled: false })
    const draft = makeConfig({ is_enabled: true })
    assert.deepEqual(mergeServerSignalConfig({ serverConfig: server, draftConfig: draft, isDirty: false, isToggling: true }), draft)
  })
})
