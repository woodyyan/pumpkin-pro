import { describe, it } from 'node:test'
import assert from 'node:assert/strict'

import {
  createQuadrantSearchState,
  findQuadrantStockByCode,
  getQuadrantSearchEntry,
  searchQuadrantStocks,
  updateQuadrantSearchEntry,
} from '../quadrant-search.js'

const ASHARE_STOCKS = [
  { c: '600519', n: '贵州茅台', o: 95.2, r: 21.1, q: '机会' },
  { c: '000001', n: '平安银行', o: 66.4, r: 33.9, q: '机会' },
]

const HKEX_STOCKS = [
  { c: '00700', n: '腾讯控股', o: 86.6, r: 27.5, q: '机会' },
  { c: '09988', n: '阿里巴巴-W', o: 72.4, r: 42.1, q: '中性' },
]

describe('quadrant per-market state flow', () => {
  it('restores each market query when switching tabs', () => {
    let state = createQuadrantSearchState()
    state = updateQuadrantSearchEntry(state, 'ASHARE', { query: '茅台', selectedCode: '600519' })
    state = updateQuadrantSearchEntry(state, 'HKEX', { query: '腾讯', selectedCode: '00700' })

    const asEntry = getQuadrantSearchEntry(state, 'ASHARE')
    const hkEntry = getQuadrantSearchEntry(state, 'HKEX')

    assert.equal(asEntry.query, '茅台')
    assert.equal(asEntry.selectedCode, '600519')
    assert.equal(hkEntry.query, '腾讯')
    assert.equal(hkEntry.selectedCode, '00700')
  })

  it('local search only targets the current market dataset', () => {
    const aResults = searchQuadrantStocks(ASHARE_STOCKS, '腾讯', 'ASHARE')
    const hkResults = searchQuadrantStocks(HKEX_STOCKS, '腾讯', 'HKEX')

    assert.equal(aResults.length, 0)
    assert.equal(hkResults.length, 1)
    assert.equal(hkResults[0].c, '00700')
  })

  it('re-hydrates selected stock from refreshed market data by code', () => {
    const selected = findQuadrantStockByCode(ASHARE_STOCKS, '600519', 'ASHARE')
    assert.ok(selected)
    assert.equal(selected.n, '贵州茅台')
  })
})
