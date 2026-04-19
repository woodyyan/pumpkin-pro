import { describe, it } from 'node:test'
import assert from 'node:assert/strict'

function formatSignalPerformancePct(value) {
  if (typeof value !== 'number' || !Number.isFinite(value)) return '--'
  const prefix = value > 0 ? '+' : ''
  return `${prefix}${value.toFixed(1)}%`
}

function getSignalPerformanceReturnClass(value) {
  if (typeof value !== 'number' || !Number.isFinite(value)) return 'text-white/40'
  return value >= 0 ? 'text-red-300' : 'text-emerald-300'
}

function hasQualityValidationReturn(validation) {
  return typeof validation?.primary_return_pct === 'number' && Number.isFinite(validation.primary_return_pct)
}

function buildQualityValidationHeadline(validation) {
  if (!validation) return ''
  const days = Math.max(1, validation.primary_window_days || 5)
  const availableDays = Math.max(0, validation.available_days || 0)
  if (validation.summary_status === 'pending') {
    return `${days}日验证中（${Math.min(availableDays, days)}/${days}）`
  }
  if (hasQualityValidationReturn(validation)) {
    return `${days}日验证：${formatSignalPerformancePct(validation.primary_return_pct)}`
  }
  return `${days}日验证`
}

function buildQualityValidationStatusLabel(validation) {
  return validation?.summary_label || ''
}

function getQualityValidationStatusClass(validation) {
  switch (validation?.summary_status) {
    case 'hit':
      return 'text-sky-200 bg-sky-500/10 border-sky-400/25'
    case 'miss':
      return 'text-rose-200 bg-rose-500/10 border-rose-400/25'
    case 'pending':
      return 'text-amber-200 bg-amber-500/10 border-amber-400/25'
    default:
      return 'text-white/55 bg-white/[0.05] border-white/10'
  }
}

function buildQualityWindowStatusLabel(window) {
  if (!window?.ready) return '验证中'
  if (window.direction_status === 'hit') return '命中'
  if (window.direction_status === 'miss') return '失准'
  return '区间变动'
}

function buildQualityWindowValue(window, validation) {
  if (window?.ready && typeof window?.return_pct === 'number' && Number.isFinite(window.return_pct)) {
    return formatSignalPerformancePct(window.return_pct)
  }
  const horizon = Math.max(1, window?.horizon_days || 0)
  const available = Math.max(0, validation?.available_days || 0)
  return `已完成 ${Math.min(available, horizon)}/${horizon}`
}

describe('quality validation headline helpers', () => {
  it('formats completed 5-day validation headline', () => {
    assert.equal(
      buildQualityValidationHeadline({ primary_window_days: 5, summary_status: 'hit', primary_return_pct: 6.18 }),
      '5日验证：+6.2%'
    )
  })

  it('formats pending validation headline with progress', () => {
    assert.equal(
      buildQualityValidationHeadline({ primary_window_days: 5, summary_status: 'pending', available_days: 2 }),
      '5日验证中（2/5）'
    )
  })

  it('falls back to plain validation label when return is absent', () => {
    assert.equal(
      buildQualityValidationHeadline({ primary_window_days: 5, summary_status: 'unknown' }),
      '5日验证'
    )
  })
})

describe('quality validation status helpers', () => {
  it('returns direct backend labels for card badges', () => {
    assert.equal(buildQualityValidationStatusLabel({ summary_label: '命中' }), '命中')
    assert.equal(buildQualityValidationStatusLabel({ summary_label: '区间变动' }), '区间变动')
  })

  it('maps statuses to presentation classes', () => {
    assert.equal(getQualityValidationStatusClass({ summary_status: 'hit' }), 'text-sky-200 bg-sky-500/10 border-sky-400/25')
    assert.equal(getQualityValidationStatusClass({ summary_status: 'miss' }), 'text-rose-200 bg-rose-500/10 border-rose-400/25')
    assert.equal(getQualityValidationStatusClass({ summary_status: 'pending' }), 'text-amber-200 bg-amber-500/10 border-amber-400/25')
    assert.equal(getQualityValidationStatusClass({ summary_status: 'unknown' }), 'text-white/55 bg-white/[0.05] border-white/10')
  })
})

describe('quality validation detail window helpers', () => {
  it('formats ready windows with signed returns and chinese-market colors', () => {
    assert.equal(buildQualityWindowStatusLabel({ ready: true, direction_status: 'hit' }), '命中')
    assert.equal(buildQualityWindowValue({ ready: true, return_pct: -4.12 }, { available_days: 5 }), '-4.1%')
    assert.equal(getSignalPerformanceReturnClass(-4.12), 'text-emerald-300')
  })

  it('formats pending windows with progress', () => {
    assert.equal(buildQualityWindowStatusLabel({ ready: false, horizon_days: 10 }), '验证中')
    assert.equal(buildQualityWindowValue({ ready: false, horizon_days: 10 }, { available_days: 5 }), '已完成 5/10')
  })

  it('keeps hold windows neutral', () => {
    assert.equal(buildQualityWindowStatusLabel({ ready: true, direction_status: 'unknown' }), '区间变动')
  })
})
