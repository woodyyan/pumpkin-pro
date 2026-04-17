import { describe, it } from 'node:test'
import assert from 'node:assert/strict'

import {
  buildSignalConfigPayload,
  canEnableSignal,
  hasSignalConfigChanged,
  mergeServerSignalConfig,
  normalizeSignalConfig,
} from '../../lib/signal-config-ui.js'

const strategies = [{ id: 'macd', name: 'MACD' }]

function makeServer(overrides = {}) {
  return normalizeSignalConfig({
    symbol: '600036.SH',
    strategy_id: 'macd',
    is_enabled: false,
    cooldown_seconds: 3600,
    eval_interval_seconds: 3600,
    thresholds: {},
    ...overrides,
  }, '600036.SH', strategies)
}

describe('signal config interaction workflow', () => {
  it('keeps unsaved draft when polling returns older server config', () => {
    const server = makeServer({ strategy_id: 'macd', eval_interval_seconds: 3600 })
    const draft = makeServer({ strategy_id: 'macd', eval_interval_seconds: 900 })

    assert.equal(hasSignalConfigChanged(server, draft), true)
    assert.deepEqual(
      mergeServerSignalConfig({ serverConfig: server, draftConfig: draft, isDirty: true, isToggling: false }),
      draft,
    )
  })

  it('keeps optimistic toggle state when polling returns stale server state', () => {
    const server = makeServer({ is_enabled: false })
    const optimisticDraft = makeServer({ is_enabled: true })

    assert.deepEqual(
      mergeServerSignalConfig({ serverConfig: server, draftConfig: optimisticDraft, isDirty: false, isToggling: true }),
      optimisticDraft,
    )
  })

  it('blocks enable action without strategy before any request is sent', () => {
    const draft = makeServer({ strategy_id: '' })
    const check = canEnableSignal(draft)
    assert.equal(check.ok, false)
    assert.equal(check.reason, '请先选择策略，再开启信号')
  })

  it('toggle payload carries current draft values when enabling', () => {
    const draft = makeServer({ strategy_id: 'macd', eval_interval_seconds: 900, thresholds: { score: 0.75 } })
    const optimisticDraft = { ...draft, is_enabled: true }

    assert.deepEqual(buildSignalConfigPayload(optimisticDraft, true), {
      strategy_id: 'macd',
      is_enabled: true,
      cooldown_seconds: 3600,
      eval_interval_seconds: 900,
      thresholds: { score: 0.75 },
    })
  })
})
