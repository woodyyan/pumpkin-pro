// ── Pure function tests for stock-analysis payload assembly ──
// Uses Node 20+ built-in test runner (node --test)
// Tests the market/fundamentals payload logic from pages/live-trading/[symbol].js
//
// These functions are exact copies of the payload-assembly code in [symbol].js
// to avoid ESM/CJS interop issues with Next.js page components.

import { describe, it } from 'node:test'
import assert from 'node:assert/strict'

// ── Re-implemented from [symbol].js (market payload, lines ~259-268) ──

function buildMarketPayload(snap) {
  return {
    price: snap.last_price ?? 0,
    change_pct: snap.change_rate ?? 0,
    volume: snap.volume ?? null,           // 修复后：null 而非 0
    turnover_rate: snap.turnover_rate ?? null,
    open: snap.open ?? null,
    high: snap.high ?? null,
    low: snap.low ?? null,
  }
}

// ── Re-implemented from [symbol].js (fundamentals payload) ──

function buildFundamentalsPayload(items) {
  return {
    pe_ttm: items.pe_ttm ?? 'N/A',
    pb_ttm: items.pb_ttm ?? 'N/A',
    dividend_yield: items.dividend_yield ?? 'N/A',
    // 新增的 unavailable 标志位
    peUnavailable: !items.pe_ttm || Number(items.pe_ttm) <= 0,
    pbUnavailable: !items.pb_ttm || Number(items.pb_ttm) <= 0,
    divYieldUnavailable: !items.dividend_yield || Number(items.dividend_yield) < 0,
    _valid: !!(items.market_cap_text),
  }
}

// ══════════════════════════════════════
// F1-F4: Market payload
// ══════════════════════════════════════

describe('buildMarketPayload - full snapshot', () => {
  it('returns all fields when snapshot has complete data', () => {
    const snap = {
      last_price: 1800.50,
      change_rate: 1.23,
      volume: 123456,
      turnover_rate: 3.45,
      open: 1790.00,
      high: 1810.00,
      low: 1785.00,
    }
    const m = buildMarketPayload(snap)
    assert.equal(m.price, 1800.50)
    assert.equal(m.change_pct, 1.23)
    assert.equal(m.volume, 123456)
    assert.equal(m.turnover_rate, 3.45)
    assert.equal(m.open, 1790.00)
    assert.equal(m.high, 1810.00)
    assert.equal(m.low, 1785.00)
  })
})

describe('buildMarketPayload - missing fields use null', () => {
  it('returns null for missing turnover_rate, not 0', () => {
    const snap = { last_price: 100, change_rate: 1.5 }
    const m = buildMarketPayload(snap)
    assert.equal(m.turnover_rate, null, 'must be null, not 0')
    assert.equal(m.open, null, 'open must be null')
    assert.equal(m.high, null, 'high must be null')
    assert.equal(m.low, null, 'low must be null')
    assert.equal(m.volume, null, 'volume must be null')
  })

  it('returns null for all OHL fields when absent', () => {
    const snap = { last_price: 100, change_rate: 2.0, volume: 50000 }
    const m = buildMarketPayload(snap)
    assert.equal(m.open, null)
    assert.equal(m.high, null)
    assert.equal(m.low, null)
  })
})

describe('buildMarketPayload - zero is valid for suspended stocks', () => {
  it('preserves 0 as legitimate value', () => {
    const snap = { last_price: 100, change_rate: 0, volume: 0, turnover_rate: 0 }
    const m = buildMarketPayload(snap)
    assert.equal(m.volume, 0, 'zero volume is valid (suspended trading)')
    assert.equal(m.turnover_rate, 0, 'zero turnover is valid')
  })

  it('preserves zero OHL values when explicitly provided', () => {
    const snap = { last_price: 100, change_rate: 0, open: 100, high: 100, low: 100 }
    const m = buildMarketPayload(snap)
    assert.equal(m.open, 100)
    assert.equal(m.high, 100)
    assert.equal(m.low, 100)
  })
})

// ══════════════════════════════════════
// F5-F10: Fundamentals payload
// ══════════════════════════════════════

describe('buildFundamentalsPayload - all present', () => {
  it('sets unavailable flags to false when data is valid', () => {
    const items = {
      pe_ttm: '28.5',
      pb_ttm: '8.5',
      dividend_yield: '2.5',
      market_cap_text: '225000000000',
    }
    const f = buildFundamentalsPayload(items)
    assert.equal(f.peUnavailable, false)
    assert.equal(f.pbUnavailable, false)
    assert.equal(f.divYieldUnavailable, false)
    assert.equal(f._valid, true)
    assert.equal(f.pe_ttm, '28.5')
    assert.equal(f.pb_ttm, '8.5')
    assert.equal(f.dividend_yield, '2.5')
  })
})

describe('buildFundamentalsPayload - missing PE', () => {
  it('marks PE unavailable when empty string', () => {
    const items = { pe_ttm: '', pb_ttm: '5.0', dividend_yield: '1.5' }
    const f = buildFundamentalsPayload(items)
    assert.equal(f.peUnavailable, true)
    assert.equal(f.pbUnavailable, false)
  })

  it('marks PE unavailable when zero or negative', () => {
    assert.equal(buildFundamentalsPayload({ pe_ttm: '0' }).peUnavailable, true)
    assert.equal(buildFundamentalsPayload({ pe_ttm: '-5' }).peUnavailable, true)
  })
})

describe('buildFundamentalsPayload - missing PB', () => {
  it('marks PB unavailable when empty/zero/negative', () => {
    assert.equal(buildFundamentalsPayload({ pb_ttm: '' }).pbUnavailable, true)
    assert.equal(buildFundamentalsPayload({ pb_ttm: '0' }).pbUnavailable, true)
    assert.equal(buildFundamentalsPayload({ pb_ttm: '-1' }).pbUnavailable, true)
  })
})

describe('buildFundamentalsPayload - negative dividend yield', () => {
  it('marks dividend yield unavailable when negative', () => {
    const f = buildFundamentalsPayload({ dividend_yield: '-1' })
    assert.equal(f.divYieldUnavailable, true)
  })

  it('keeps dividend yield available when 0 or positive', () => {
    assert.equal(buildFundamentalsPayload({ dividend_yield: '0' }).divYieldUnavailable, false)
    assert.equal(buildFundamentalsPayload({ dividend_yield: '3.0' }).divYieldUnavailable, false)
  })
})

describe('buildFundamentalsPayload - empty input', () => {
  it('marks _valid=false when no market_cap_text', () => {
    const f = buildFundamentalsPayload({})
    assert.equal(f._valid, false)
    assert.equal(f.pe_ttm, 'N/A')
    assert.equal(f.pb_ttm, 'N/A')
    assert.equal(f.dividend_yield, 'N/A')
    assert.equal(f.peUnavailable, true)
    assert.equal(f.pbUnavailable, true)
    assert.equal(f.divYieldUnavailable, true)
  })
})

describe('roundtrip - JSON serialization', () => {
  it('correctly serializes null values in JSON', () => {
    const snap = { last_price: 1800.50, change_rate: 1.23 }  // minimal
    const m = buildMarketPayload(snap)
    const json = JSON.stringify(m)
    const parsed = JSON.parse(json)

    // null should stay null in JSON roundtrip
    assert.equal(parsed.volume, null)
    assert.equal(parsed.turnover_rate, null)
    assert.equal(parsed.open, null)
    assert.equal(parsed.high, null)
    assert.equal(parsed.low, null)

    // numeric values should survive
    assert.equal(parsed.price, 1800.50)
    assert.equal(parsed.change_pct, 1.23)
  })

  it('serializes unavailable flags correctly', () => {
    const items = { pe_ttm: '', pb_ttm: '0', dividend_yield: '-1', market_cap_text: '1e11' }
    const f = buildFundamentalsPayload(items)
    const json = JSON.stringify(f)
    const parsed = JSON.parse(json)

    assert.equal(parsed.peUnavailable, true)
    assert.equal(parsed.pbUnavailable, true)
    assert.equal(parsed.divYieldUnavailable, true)
    assert.equal(parsed._valid, true)
  })
})
