import { describe, it } from 'node:test'
import assert from 'node:assert/strict'

import {
  QUADRANT_SEARCH_MAX_RESULTS,
  QUADRANT_SEARCH_MIN_QUERY_LEN,
  buildQuadrantDetailSymbol,
  createQuadrantSearchState,
  findQuadrantStockByCode,
  getQuadrantSearchEntry,
  normalizeQuadrantStockCode,
  searchQuadrantStocks,
  updateQuadrantSearchEntry,
} from '../quadrant-search.js'

const A_STOCKS = [
  { c: '600519', n: '贵州茅台', o: 96.2, r: 18.3, q: '机会' },
  { c: '600516', n: '贵州燃气', o: 71.5, r: 36.2, q: '中性' },
  { c: '000001', n: '平安银行', o: 68.2, r: 32.5, q: '机会' },
  { c: '300750', n: '宁德时代', o: 88.1, r: 41.7, q: '拥挤' },
]

const HK_STOCKS = [
  { c: '00700', n: '腾讯控股', o: 84.8, r: 28.4, q: '机会' },
  { c: '09988', n: '阿里巴巴-W', o: 76.5, r: 39.7, q: '中性' },
  { c: '00005', n: '汇丰控股', o: 62.1, r: 24.2, q: '防御' },
]

describe('quadrant search helpers', () => {
  it('uses stable local-search thresholds', () => {
    assert.equal(QUADRANT_SEARCH_MIN_QUERY_LEN, 2)
    assert.equal(QUADRANT_SEARCH_MAX_RESULTS, 8)
  })

  it('keeps A/H market state independent', () => {
    let state = createQuadrantSearchState()
    state = updateQuadrantSearchEntry(state, 'ASHARE', { query: '茅台', selectedCode: '600519' })
    state = updateQuadrantSearchEntry(state, 'HKEX', { query: '腾讯', selectedCode: '00700' })

    assert.deepEqual(getQuadrantSearchEntry(state, 'ASHARE'), { query: '茅台', selectedCode: '600519' })
    assert.deepEqual(getQuadrantSearchEntry(state, 'HKEX'), { query: '腾讯', selectedCode: '00700' })
  })

  it('finds A-share by exact code first', () => {
    const results = searchQuadrantStocks(A_STOCKS, '600519', 'ASHARE')
    assert.equal(results[0].c, '600519')
    assert.equal(results[0].n, '贵州茅台')
  })

  it('finds HK stock by short code without leading zeros', () => {
    const results = searchQuadrantStocks(HK_STOCKS, '700', 'HKEX')
    assert.equal(results[0].c, '00700')
    assert.equal(results[0].n, '腾讯控股')
  })

  it('supports exact Chinese name matching', () => {
    const results = searchQuadrantStocks(A_STOCKS, '平安银行', 'ASHARE')
    assert.equal(results[0].c, '000001')
  })

  it('supports fuzzy Chinese matching', () => {
    const results = searchQuadrantStocks(HK_STOCKS, '腾讯', 'HKEX')
    assert.equal(results.length, 1)
    assert.equal(results[0].c, '00700')
  })

  it('returns empty results for short queries', () => {
    assert.deepEqual(searchQuadrantStocks(A_STOCKS, '茅', 'ASHARE'), [])
    assert.deepEqual(searchQuadrantStocks(HK_STOCKS, '7', 'HKEX'), [])
  })

  it('respects result limit', () => {
    const results = searchQuadrantStocks(A_STOCKS, '0', 'ASHARE', 2)
    assert.equal(results.length, 0, 'short query still gated before limit applies')

    const fuzzy = searchQuadrantStocks(A_STOCKS, '00', 'ASHARE', 2)
    assert.equal(fuzzy.length, 2)
  })

  it('findQuadrantStockByCode uses market-specific normalization', () => {
    assert.equal(findQuadrantStockByCode(A_STOCKS, '1', 'ASHARE')?.c, '000001')
    assert.equal(findQuadrantStockByCode(HK_STOCKS, '700', 'HKEX')?.c, '00700')
  })

  it('buildQuadrantDetailSymbol preserves 5-digit HK codes', () => {
    assert.equal(buildQuadrantDetailSymbol('00700', 'HKEX'), '00700.HK')
    assert.equal(buildQuadrantDetailSymbol('700', 'HKEX'), '00700.HK')
  })

  it('buildQuadrantDetailSymbol maps A-share codes to SH/SZ correctly', () => {
    assert.equal(buildQuadrantDetailSymbol('600519', 'ASHARE'), '600519.SH')
    assert.equal(buildQuadrantDetailSymbol('000001', 'ASHARE'), '000001.SZ')
    assert.equal(buildQuadrantDetailSymbol('300750', 'ASHARE'), '300750.SZ')
  })

  it('normalizes codes by market width', () => {
    assert.equal(normalizeQuadrantStockCode('700', 'HKEX'), '00700')
    assert.equal(normalizeQuadrantStockCode('1', 'ASHARE'), '000001')
  })
})
