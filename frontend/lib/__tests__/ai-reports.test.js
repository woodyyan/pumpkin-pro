import { describe, it } from 'node:test'
import assert from 'node:assert/strict'

import { AI_REPORT_PRICING_PLANS, getAIReportMarketLabel, normalizeAIReportItems } from '../ai-reports.js'

describe('ai report helpers', () => {
  it('keeps first phase pricing static', () => {
    assert.deepEqual(AI_REPORT_PRICING_PLANS.map((plan) => plan.price), ['9.9 元', '39 元', '69 元'])
    assert.deepEqual(AI_REPORT_PRICING_PLANS.map((plan) => plan.quota), ['1 份', '5 份', '10 份'])
  })

  it('formats supported market labels', () => {
    assert.equal(getAIReportMarketLabel('SSE'), 'A股 · 上交所')
    assert.equal(getAIReportMarketLabel('SZSE'), 'A股 · 深交所')
    assert.equal(getAIReportMarketLabel('HKEX'), '中国香港股票')
  })

  it('normalizes report list items and drops invalid rows', () => {
    const items = normalizeAIReportItems([
      { id: 'r1', stock_name: '腾讯控股', symbol: '00700', exchange: 'hkex', source_trade_date: '2026-06-26', thumbnail_url: 'thumb.webp' },
      { id: '', stock_name: '无效', symbol: '000001' },
    ])

    assert.equal(items.length, 1)
    assert.deepEqual(items[0], {
      id: 'r1',
      stockName: '腾讯控股',
      symbol: '00700',
      exchange: 'HKEX',
      sourceTradeDate: '2026-06-26',
      thumbnailURL: 'thumb.webp',
    })
  })
})
