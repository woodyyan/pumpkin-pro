import { describe, it } from 'node:test'
import assert from 'node:assert/strict'

import { AI_REPORT_PRICING_PLANS, getAIReportMarketLabel, normalizeAIReportItems } from '../ai-reports.js'

describe('ai report helpers', () => {
  it('keeps first phase pricing static', () => {
    assert.deepEqual(AI_REPORT_PRICING_PLANS.map((plan) => plan.name), ['体验版', '入门版', '投资版', '专业版'])
    assert.deepEqual(AI_REPORT_PRICING_PLANS.map((plan) => plan.price), ['9.9 元', '39 元', '69 元', '199 元'])
    assert.deepEqual(AI_REPORT_PRICING_PLANS.map((plan) => plan.quota), ['1 份', '5 份', '10 份', '50 份'])
    assert.deepEqual(AI_REPORT_PRICING_PLANS.map((plan) => plan.unitPrice), ['9.9 元/份', '7.8 元/份', '6.9 元/份', '3.98 元/份'])
    assert.equal(AI_REPORT_PRICING_PLANS[0].reportType, '标准 PDF 报告')
    assert.equal(AI_REPORT_PRICING_PLANS[2].badge, '推荐')
    assert.equal(AI_REPORT_PRICING_PLANS[2].featured, true)
    assert.equal(AI_REPORT_PRICING_PLANS[1].badge, undefined)
    assert.deepEqual(AI_REPORT_PRICING_PLANS[0].highlights, ['1 份标准 PDF 报告'])
    assert.deepEqual(AI_REPORT_PRICING_PLANS[1].highlights, ['5 份标准 PDF 报告'])
    assert.ok(AI_REPORT_PRICING_PLANS[2].highlights.includes('10 份标准 PDF 报告'))
    assert.ok(AI_REPORT_PRICING_PLANS[2].highlights.includes('支持自定义分析问题'))
    assert.ok(AI_REPORT_PRICING_PLANS[3].highlights.includes('50 份标准 PDF 报告'))
    assert.ok(AI_REPORT_PRICING_PLANS[3].highlights.includes('多股票对比分析'))
  })

  it('keeps custom question and multi-stock comparison examples', () => {
    const investmentPlan = AI_REPORT_PRICING_PLANS.find((plan) => plan.key === 'investment')
    const professionalPlan = AI_REPORT_PRICING_PLANS.find((plan) => plan.key === 'professional')

    assert.ok(investmentPlan.examples.includes('我的成本 42 元，要不要止损？'))
    assert.ok(investmentPlan.examples.includes('为什么该股票近一个月一直跌？'))
    assert.ok(professionalPlan.examples.includes('帮我比较小米 vs 比亚迪 vs 宁德时代的股票'))
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
