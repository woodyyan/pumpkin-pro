import { describe, it } from 'node:test'
import assert from 'node:assert/strict'

import {
  buildInsightText,
  buildNewsKlineUrl,
  changeClassName,
  chartPalette,
  filterKlineByRange,
  formatPercent,
  symbolFromSearchResult,
} from '../news-kline.js'


describe('news kline helpers', () => {
  it('builds the confirmed backend route with URLSearchParams', () => {
    assert.equal(
      buildNewsKlineUrl({ symbol: '600519.SH', days: 500, pages: 3, force: true }),
      '/api/live/news-kline?symbol=600519.SH&days=500&pages=3&force=1',
    )
  })

  it('normalizes search results to live symbols', () => {
    assert.equal(symbolFromSearchResult({ code: '600519', exchange: 'SSE' }), '600519.SH')
    assert.equal(symbolFromSearchResult({ code: '000001', exchange: 'SZSE' }), '000001.SZ')
    assert.equal(symbolFromSearchResult({ code: '700', exchange: 'HKEX' }), '00700.HK')
  })

  it('formats returns using red-up green-down classes', () => {
    assert.equal(formatPercent(0.1234), '+12.3%')
    assert.equal(formatPercent(-0.012), '-1.2%')
    assert.equal(changeClassName(0.01), 'text-negative')
    assert.equal(changeClassName(-0.01), 'text-positive')
  })

  it('filters kline ranges safely', () => {
    const rows = [
      { date: '2025-07-16', close: 1 },
      { date: '2026-06-16', close: 2 },
      { date: '2026-07-16', close: 3 },
    ]
    assert.deepEqual(filterKlineByRange(rows, '1M').map((item) => item.date), ['2026-06-16', '2026-07-16'])
    assert.equal(filterKlineByRange(rows, 'ALL').length, 3)
  })

  it('returns distinct chart palettes and readable insight', () => {
    assert.notEqual(chartPalette('light').axis, chartPalette('dark').axis)
    const text = buildInsightText({
      META: { name: '贵州茅台' },
      STATS: [{ category: '财报业绩', count: 2, avg_3d: 0.03, win_3d: 0.5 }],
    })
    assert.match(text, /贵州茅台/)
    assert.match(text, /财报业绩/)
  })
})
