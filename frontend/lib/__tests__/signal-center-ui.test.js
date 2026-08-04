import { describe, it } from 'node:test'
import assert from 'node:assert/strict'

import {
  barStateMeta,
  buildCreateSubscriptionPayload,
  buildOverviewStats,
  buildParamsSummary,
  buildTemplateParamDraft,
  evalIntervalLabel,
  evalModeLabel,
  filterSignalEvents,
  groupSubscriptionsBySymbol,
  groupTemplatesByCategory,
  normalizeSignalEvents,
  normalizeSubscriptions,
  sideMeta,
  validateTemplateParamDraft,
} from '../signal-center-ui.js'

const macdTemplate = {
  key: 'macd_cross',
  name: 'MACD 金叉死叉',
  category: 'indicator',
  param_schema: [
    { key: 'fast_period', label: '快线周期', required: true, default: 12, min: 2, max: 60 },
    { key: 'slow_period', label: '慢线周期', required: true, default: 26, min: 5, max: 120 },
  ],
  default_params: { fast_period: 12, slow_period: 26 },
}

const priceTemplate = {
  key: 'price_above',
  name: '涨破提醒',
  category: 'price_alert',
  param_schema: [
    { key: 'price', label: '目标价', required: true, min: 0.01, unit: '元' },
  ],
  default_params: {},
}

describe('normalizeSubscriptions', () => {
  it('fills defaults for missing fields', () => {
    const [sub] = normalizeSubscriptions([{ id: 's1', template_key: 'macd_cross', symbol: '600519.SH', is_enabled: 1 }])
    assert.equal(sub.is_enabled, true)
    assert.equal(sub.eval_interval_seconds, 900)
    assert.equal(sub.cooldown_seconds, 3600)
    assert.equal(sub.position_aware_enabled, true)
  })

  it('returns empty array for invalid input', () => {
    assert.deepEqual(normalizeSubscriptions(null), [])
    assert.deepEqual(normalizeSubscriptions('x'), [])
  })
})

describe('groupSubscriptionsBySymbol', () => {
  it('groups multiple templates under one symbol and counts enabled', () => {
    const groups = groupSubscriptionsBySymbol([
      { id: '1', symbol: '600519.SH', symbol_name: '贵州茅台', template_key: 'macd_cross', is_enabled: true },
      { id: '2', symbol: '600519.SH', symbol_name: '贵州茅台', template_key: 'rsi_range', is_enabled: false },
      { id: '3', symbol: '00700.HK', symbol_name: '腾讯控股', template_key: 'price_above', is_enabled: true },
    ])
    assert.equal(groups.length, 2)
    const maotai = groups.find((g) => g.symbol === '600519.SH')
    assert.equal(maotai.items.length, 2)
    assert.equal(maotai.enabled_count, 1)
    assert.equal(maotai.symbol_name, '贵州茅台')
  })

  it('skips entries without symbol', () => {
    const groups = groupSubscriptionsBySymbol([{ id: '1', symbol: '', template_key: 'macd_cross' }])
    assert.equal(groups.length, 0)
  })
})

describe('buildParamsSummary', () => {
  it('renders label + value + unit via schema', () => {
    assert.equal(buildParamsSummary(priceTemplate, { price: 100 }), '目标价 100元')
    assert.equal(buildParamsSummary(macdTemplate, { fast_period: 10 }), '快线周期 10 · 慢线周期 26')
  })

  it('returns empty string without schema', () => {
    assert.equal(buildParamsSummary({}, { a: 1 }), '')
  })
})

describe('evalIntervalLabel / evalModeLabel', () => {
  it('maps known intervals', () => {
    assert.equal(evalIntervalLabel(900), '每 15 分钟')
    assert.equal(evalIntervalLabel(14400), '每 4 小时')
  })

  it('labels price alert as realtime and volume_breakout as close-only', () => {
    assert.equal(evalModeLabel({ category: 'price_alert' }), '盘中实时')
    assert.equal(evalModeLabel({ category: 'indicator', template_key: 'volume_breakout' }), '收盘确认')
    assert.equal(evalModeLabel({ category: 'indicator', template_key: 'macd_cross', eval_interval_seconds: 1800 }), '每 30 分钟')
  })
})

describe('barStateMeta / sideMeta', () => {
  it('distinguishes provisional vs confirmed vs realtime', () => {
    assert.equal(barStateMeta('intraday_provisional').label, '盘中试算')
    assert.equal(barStateMeta('close_confirmed').label, '收盘确认')
    assert.equal(barStateMeta('realtime').label, '实时触发')
    assert.equal(barStateMeta('').label, '')
  })

  it('uses A-share red-buy green-sell semantics', () => {
    assert.equal(sideMeta('BUY').tone, 'rise')
    assert.equal(sideMeta('SELL').tone, 'fall')
  })
})

describe('filterSignalEvents', () => {
  const events = normalizeSignalEvents([
    { event_id: 'e1', symbol: '600519.SH', side: 'BUY', bar_state: 'intraday_provisional', reason: { message: 'm1' } },
    { event_id: 'e2', symbol: '600519.SH', side: 'BUY', bar_state: 'close_confirmed', reason: { message: 'm2' } },
    { event_id: 'e3', symbol: '00700.HK', side: 'SELL', bar_state: 'realtime', reason: { message: 'm3' } },
  ])

  it('filters by symbol / side / bar_state independently', () => {
    assert.equal(filterSignalEvents(events, { symbol: '600519.SH' }).length, 2)
    assert.equal(filterSignalEvents(events, { side: 'sell' }).length, 1)
    assert.equal(filterSignalEvents(events, { barState: 'close_confirmed' }).length, 1)
    assert.equal(filterSignalEvents(events, {}).length, 3)
  })

  it('extracts message from reason', () => {
    const [first] = filterSignalEvents(events, {})
    assert.equal(first.message, 'm1')
  })
})

describe('buildOverviewStats', () => {
  it('counts enabled subscriptions and today events by trade_date', () => {
    const stats = buildOverviewStats(
      [
        { id: '1', symbol: '600519.SH', is_enabled: true },
        { id: '2', symbol: '00700.HK', is_enabled: false },
      ],
      [
        { event_id: 'e1', symbol: '600519.SH', trade_date: '2026-08-04' },
        { event_id: 'e2', symbol: '600519.SH', trade_date: '2026-08-03' },
      ],
      5,
      '2026-08-04',
    )
    assert.equal(stats.enabledCount, 1)
    assert.equal(stats.totalCount, 2)
    assert.equal(stats.todayCount, 1)
    assert.equal(stats.unreadCount, 5)
  })
})

describe('buildCreateSubscriptionPayload', () => {
  it('creates enabled-by-default payload with symbol scope', () => {
    const payload = buildCreateSubscriptionPayload({ templateKey: 'macd_cross', symbol: '600519.SH', params: { fast_period: 10 } })
    assert.equal(payload.is_enabled, true)
    assert.equal(payload.scope_type, 'symbol')
    assert.equal(payload.eval_interval_seconds, 900)
    assert.deepEqual(payload.params, { fast_period: 10 })
  })
})

describe('template param draft helpers', () => {
  it('builds draft from defaults', () => {
    assert.deepEqual(buildTemplateParamDraft(macdTemplate), { fast_period: 12, slow_period: 26 })
    assert.deepEqual(buildTemplateParamDraft(priceTemplate), {})
  })

  it('validates required and range', () => {
    assert.equal(validateTemplateParamDraft(priceTemplate, {}), '请填写「目标价」')
    assert.equal(validateTemplateParamDraft(priceTemplate, { price: 'abc' }), '「目标价」必须是数字')
    assert.equal(validateTemplateParamDraft(macdTemplate, { fast_period: 1, slow_period: 26 }), '「快线周期」不能小于 2')
    assert.equal(validateTemplateParamDraft(macdTemplate, { fast_period: 12, slow_period: 26 }), '')
  })
})

describe('groupTemplatesByCategory', () => {
  it('groups in fixed order and drops empty categories', () => {
    const grouped = groupTemplatesByCategory([
      { key: 'macd_cross', category: 'indicator' },
      { key: 'price_above', category: 'price_alert' },
    ])
    assert.equal(grouped.length, 2)
    assert.equal(grouped[0].key, 'price_alert')
    assert.equal(grouped[1].key, 'indicator')
  })
})
