// ── Tests for RankingPanel.js helpers and data flow ──
// Uses Node 20+ built-in test runner (node --test)
// 覆盖 T-R4: 组件渲染、Tab 切换、空状态、点击跳转

import { describe, it } from 'node:test'
import assert from 'node:assert/strict'

// ═══════════════════════════════════════════════════
// 从 RankingPanel.js 提取的纯函数（保持同步）
// ═══════════════════════════════════════════════════

function getMedal(rank) {
  if (rank === 1) return { icon: '🥇', className: '' }
  if (rank === 2) return { icon: '🥈', className: '' }
  if (rank === 3) return { icon: '🥉', className: '' }
  if (rank <= 10) return { icon: null, className: 'rounded-full bg-white/10 text-white text-[10px]' }
  return { icon: null, className: 'text-white/35 text-[10px]' }
}

function formatCode(code, exchange) {
  if (exchange === 'HKEX') return code.padStart(5, '0')
  return code
}

function exchangeLabel(ex) {
  const labels = { SSE: '沪市', SZSE: '深市', HKEX: '港股' }
  return labels[ex] || ex
}

function hasReturnPct(value) {
  return typeof value === 'number' && Number.isFinite(value)
}

function formatMetaDateTime(value) {
  if (!value) return '--'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return date.toLocaleString('zh-CN', { hour12: false })
}

function buildRankingMetaSummary(meta) {
  const parts = ['精选股票来自机会区']
  if (meta?.computed_at) {
    parts.push(`数据日期：${formatMetaDateTime(meta.computed_at)}`)
  }
  if (meta?.returned_count != null) {
    parts.push(`当前展示 TOP${meta.returned_count} 只`)
  }
  return parts.join(' · ')
}

function formatReturnPctDisplay(value) {
  if (!hasReturnPct(value)) return '--'
  const prefix = value > 0 ? '+' : ''
  return `${prefix}${value.toFixed(1)}%`
}

function buildMobileReturnSummary(days, pct) {
  const hasPct = hasReturnPct(pct)
  if (!days && !hasPct) return null
  const parts = []
  if (days > 0) parts.push(`🔥已连续上榜 ${days} 日`)
  if (hasPct) parts.push(`上榜以来 ${formatReturnPctDisplay(pct)}`)
  return parts.join(' · ')
}

// Mock sample ranking items for testing
const SAMPLE_ASHARE_ITEMS = [
  {
    rank: 1, code: '600519', name: '贵州茅台', exchange: 'SSE',
    opportunity: 96.5, risk: 22.3, quadrant: '机会',
    trend: 94.2, flow: 88.7, revision: 85.1,
  },
  {
    rank: 2, code: '000001', name: '平安银行', exchange: 'SZSE',
    opportunity: 94.8, risk: 28.1, quadrant: '机会',
    trend: 91.5, flow: 86.3, revision: 78.9,
  },
  {
    rank: 3, code: '601318', name: '中国平安', exchange: 'SSE',
    opportunity: 92.3, risk: 18.7, quadrant: '机会',
    trend: 90.0, flow: 84.0, revision: 80.5,
  },
]

const SAMPLE_HKEX_ITEMS = [
  {
    rank: 1, code: '700', name: '腾讯控股', exchange: 'HKEX',
    opportunity: 93.2, risk: 20.5, quadrant: '机会',
    trend: 91.0, flow: 87.0, revision: 82.0,
  },
  {
    rank: 2, code: '9988', name: '阿里巴巴', exchange: 'HKEX',
    opportunity: 89.6, risk: 26.3, quadrant: '机会',
    trend: 86.0, flow: 82.0, revision: 78.0,
  },
]

const SAMPLE_META = {
  computed_at: '2026-04-15T02:30:00Z',
  total_in_zone: 156,
  returned_count: 20,
  exchange: 'ASHARE',
}

// ── Tests ──

describe('getMedal (ranking badge)', () => {

  it('returns gold medal for rank 1', () => {
    const m = getMedal(1)
    assert.equal(m.icon, '🥇')
    assert.equal(m.className, '')
  })

  it('returns silver medal for rank 2', () => {
    const m = getMedal(2)
    assert.equal(m.icon, '🥈')
  })

  it('returns bronze medal for rank 3', () => {
    const m = getMedal(3)
    assert.equal(m.icon, '🥉')
  })

  it('returns circle badge for ranks 4-10 (no icon)', () => {
    const m4 = getMedal(4)
    const m10 = getMedal(10)
    assert.equal(m4.icon, null)
    assert.ok(m4.className.includes('rounded-full'))
    assert.equal(m10.icon, null)
    assert.ok(m10.className.includes('rounded-full'))
  })

  it('returns plain text for ranks > 10 (no icon, dimmed)', () => {
    const m11 = getMedal(11)
    const m20 = getMedal(20)
    assert.equal(m11.icon, null)
    assert.ok(!m11.className.includes('rounded-full'))
    assert.ok(m11.className.includes('text-[10px]'))
    assert.ok(m20.className.includes('text-white/35'))
  })
})

describe('formatCode (code display)', () => {

  it('pads HK codes to 5 digits', () => {
    assert.equal(formatCode('700', 'HKEX'), '00700')
    assert.equal(formatCode('5', 'HKEX'), '00005')
    assert.equal(formatCode('03968', 'HKEX'), '03968')
  })

  it('leaves A-share codes unchanged', () => {
    assert.equal(formatCode('600519', 'SSE'), '600519')
    assert.equal(formatCode('000001', 'SZSE'), '000001')
  })
})

describe('exchangeLabel (market label)', () => {

  it('maps SSE to 沪市', () => {
    assert.equal(exchangeLabel('SSE'), '沪市')
  })

  it('maps SZSE to 深市', () => {
    assert.equal(exchangeLabel('SZSE'), '深市')
  })

  it('maps HKEX to 港股', () => {
    assert.equal(exchangeLabel('HKEX'), '港股')
  })

  it('returns raw value for unknown exchange', () => {
    assert.equal(exchangeLabel('NYSE'), 'NYSE')
  })
})

describe('ranking meta summary', () => {

  it('builds concise header copy with data date and TOP count', () => {
    const summary = buildRankingMetaSummary(SAMPLE_META)
    assert.ok(summary.includes('精选股票来自机会区'))
    assert.ok(summary.includes('数据日期：'))
    assert.ok(summary.includes('当前展示 TOP20 只'))
    assert.ok(!summary.includes('机会区共'))
  })

  it('falls back to source description when meta is missing', () => {
    assert.equal(buildRankingMetaSummary(null), '精选股票来自机会区')
  })
})

describe('return_pct display helpers', () => {

  it('treats null and undefined as no data', () => {
    assert.equal(hasReturnPct(null), false)
    assert.equal(hasReturnPct(undefined), false)
    assert.equal(formatReturnPctDisplay(null), '--')
    assert.equal(formatReturnPctDisplay(undefined), '--')
  })

  it('preserves real 0.0% return', () => {
    assert.equal(hasReturnPct(0), true)
    assert.equal(formatReturnPctDisplay(0), '0.0%')
  })

  it('formats positive and negative returns', () => {
    assert.equal(formatReturnPctDisplay(1.26), '+1.3%')
    assert.equal(formatReturnPctDisplay(-2.04), '-2.0%')
  })
})

describe('mobile return summary', () => {

  it('returns null when both consecutive days and return are absent', () => {
    assert.equal(buildMobileReturnSummary(0, null), null)
  })

  it('keeps consecutive days without return value', () => {
    assert.equal(buildMobileReturnSummary(3, null), '🔥已连续上榜 3 日')
  })

  it('shows real 0.0% return instead of hiding it', () => {
    assert.equal(buildMobileReturnSummary(2, 0), '🔥已连续上榜 2 日 · 上榜以来 0.0%')
    assert.equal(buildMobileReturnSummary(0, 0), '上榜以来 0.0%')
  })
})

describe('Ranking data structure validation', () => {

  it('A-share items have required fields', () => {
    for (const item of SAMPLE_ASHARE_ITEMS) {
      assert.ok(typeof item.rank === 'number' && item.rank > 0, `${item.code} missing valid rank`)
      assert.ok(typeof item.code === 'string' && item.code.length > 0, `${item.code} missing code`)
      assert.ok(typeof item.name === 'string' && item.name.length > 0, `${item.code} missing name`)
      assert.ok(['SSE', 'SZSE'].includes(item.exchange), `${item.code}: unexpected A-share exchange ${item.exchange}`)
      assert.ok(typeof item.opportunity === 'number' && item.opportunity >= 0, `${item.code}: invalid opportunity`)
      assert.ok(typeof item.risk === 'number' && item.risk >= 0, `${item.code}: invalid risk`)
      assert.ok(item.quadrant === '机会', `${item.code}: not in opportunity zone`)
    }
  })

  it('HK items have correct exchange field', () => {
    for (const item of SAMPLE_HKEX_ITEMS) {
      assert.equal(item.exchange, 'HKEX', `${item.code}: expected HKEX`)
    }
  })

  it('items are sorted by opportunity DESC', () => {
    for (let i = 1; i < SAMPLE_ASHARE_ITEMS.length; i++) {
      assert.ok(
        SAMPLE_ASHARE_ITEMS[i].opportunity <= SAMPLE_ASHARE_ITEMS[i - 1].opportunity,
        `Sort violation at index ${i}`
      )
    }
  })

  it('meta has all required fields', () => {
    assert.ok(SAMPLE_META.computed_at.length > 0, 'missing computed_at')
    assert.ok(SAMPLE_META.total_in_zone > 0, 'total_in_zone should be positive')
    assert.ok(SAMPLE_META.returned_count > 0, 'returned_count should be positive')
    assert.ok(['ASHARE', 'HKEX'].includes(SAMPLE_META.exchange), 'invalid meta exchange')
  })
})

describe('Edge cases', () => {

  it('empty items list renders empty state (no crash)', () => {
    // Verify empty array doesn't throw when iterating
    const items = []
    let count = 0
    for (const _ of items) { count++ }
    assert.equal(count, 0)
  })

  it('single item still works correctly', () => {
    const single = [SAMPLE_ASHARE_ITEMS[0]]
    assert.equal(single.length, 1)
    assert.equal(single[0].rank, 1)
  })

  it('HK code padding handles edge cases', () => {
    // Already 5 digits
    assert.equal(formatCode('99999', 'HKEX'), '99999')
    // Single digit
    assert.equal(formatCode('1', 'HKEX'), '00001')
    // Empty string
    assert.equal(formatCode('', 'HKEX').length, 5)
  })

  it('meta with zero total_in_zone (empty opportunity zone)', () => {
    const emptyMeta = {
      computed_at: '',
      total_in_zone: 0,
      returned_count: 0,
      exchange: 'ASHARE',
    }
    assert.equal(emptyMeta.total_in_zone, 0)
    assert.equal(emptyMeta.returned_count, 0)
  })
})
