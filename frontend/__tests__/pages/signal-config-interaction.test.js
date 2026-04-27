import { describe, it } from 'node:test'
import assert from 'node:assert/strict'

import {
  buildSignalConfigMeta,
  buildSignalConfigPayload,
  buildSignalStatusSummary,
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

  it('builds folded summary content without exposing inline switch state', () => {
    const draft = makeServer({ is_enabled: true, eval_interval_seconds: 900 })
    const summary = buildSignalStatusSummary({ config: draft })
    const meta = buildSignalConfigMeta({
      config: draft,
      strategyMap: { macd: { name: 'MACD' } },
      isDirty: false,
      webhookConfigured: true,
      webhookEnabled: true,
    })

    assert.equal(summary, '交易信号已开启')
    assert.deepEqual(meta, [
      { label: '状态', value: '已开启' },
      { label: '策略', value: 'MACD' },
      { label: '频率', value: '每 15 分钟' },
      { label: '推送', value: '已就绪' },
    ])
  })

  it('surfaces unsaved and webhook risk in folded summary meta', () => {
    const draft = makeServer({ is_enabled: true, strategy_id: '' })
    const meta = buildSignalConfigMeta({
      config: draft,
      strategyMap: {},
      isDirty: true,
      webhookConfigured: false,
      webhookEnabled: false,
    })

    assert.equal(meta[3].value, '未就绪')
    assert.deepEqual(meta[4], { label: '配置', value: '有未保存修改', tone: 'warning' })
  })
})