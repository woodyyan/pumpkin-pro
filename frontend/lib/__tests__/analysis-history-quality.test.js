import { describe, it } from 'node:test'
import assert from 'node:assert/strict'

function hasSignalPerformanceReturn(perf) {
  return typeof perf?.return_pct === 'number' && Number.isFinite(perf.return_pct)
}

function formatSignalPerformancePct(value) {
  if (typeof value !== 'number' || !Number.isFinite(value)) return '--'
  const prefix = value > 0 ? '+' : ''
  return `${prefix}${value.toFixed(1)}%`
}

function getSignalPerformanceReturnClass(value) {
  if (typeof value !== 'number' || !Number.isFinite(value)) return 'text-white/40'
  return value >= 0 ? 'text-red-300' : 'text-emerald-300'
}

function buildSignalPerformanceSummary(signal, perf) {
  if (!hasSignalPerformanceReturn(perf)) return ''
  const pct = formatSignalPerformancePct(perf.return_pct)
  if (signal === 'buy') return `自上次看多以来，涨幅 ${pct}`
  if (signal === 'sell') return `自上次看空以来，区间表现 ${pct}`
  return `自上次观望以来，区间变动 ${pct}`
}

function buildSignalPerformanceStatus(signal, perf) {
  if (!perf?.direction_status || signal === 'hold') return ''
  return perf.direction_status === 'aligned' ? '与观点一致' : '与观点相反'
}

function isSignalPerformanceEstimated(perf) {
  return perf?.price_basis === 'estimated_close' || perf?.price_basis === 'mixed'
}

function formatAnalysisHistoryPrice(value, symbol) {
  if (typeof value !== 'number' || !Number.isFinite(value) || value <= 0) return '--'
  const prefix = String(symbol || '').endsWith('.HK') ? 'HK$' : '¥'
  return `${prefix}${value.toFixed(2)}`
}

describe('signal performance summary helpers', () => {
  it('formats buy summary with signed return', () => {
    assert.equal(
      buildSignalPerformanceSummary('buy', { return_pct: 8.24 }),
      '自上次看多以来，涨幅 +8.2%'
    )
  })

  it('formats sell summary without rewriting raw sign', () => {
    assert.equal(
      buildSignalPerformanceSummary('sell', { return_pct: -6.41 }),
      '自上次看空以来，区间表现 -6.4%'
    )
  })

  it('formats hold summary as neutral movement', () => {
    assert.equal(
      buildSignalPerformanceSummary('hold', { return_pct: 1.04 }),
      '自上次观望以来，区间变动 +1.0%'
    )
  })

  it('returns empty summary when return is absent', () => {
    assert.equal(buildSignalPerformanceSummary('buy', null), '')
  })
})

describe('signal performance status helpers', () => {
  it('marks aligned and opposite statuses for directional views', () => {
    assert.equal(buildSignalPerformanceStatus('buy', { direction_status: 'aligned' }), '与观点一致')
    assert.equal(buildSignalPerformanceStatus('sell', { direction_status: 'opposite' }), '与观点相反')
  })

  it('keeps hold status empty', () => {
    assert.equal(buildSignalPerformanceStatus('hold', { direction_status: 'aligned' }), '')
  })
})

describe('signal performance visual helpers', () => {
  it('maps return sign to chinese-market colors', () => {
    assert.equal(getSignalPerformanceReturnClass(0), 'text-red-300')
    assert.equal(getSignalPerformanceReturnClass(2.1), 'text-red-300')
    assert.equal(getSignalPerformanceReturnClass(-2.1), 'text-emerald-300')
    assert.equal(getSignalPerformanceReturnClass(null), 'text-white/40')
  })

  it('flags estimated or mixed price basis', () => {
    assert.equal(isSignalPerformanceEstimated({ price_basis: 'analysis' }), false)
    assert.equal(isSignalPerformanceEstimated({ price_basis: 'estimated_close' }), true)
    assert.equal(isSignalPerformanceEstimated({ price_basis: 'mixed' }), true)
  })

  it('formats mainland and hk prices', () => {
    assert.equal(formatAnalysisHistoryPrice(23.456, '000001.SZ'), '¥23.46')
    assert.equal(formatAnalysisHistoryPrice(388.6, '00700.HK'), 'HK$388.60')
    assert.equal(formatAnalysisHistoryPrice(0, '000001.SZ'), '--')
  })
})
