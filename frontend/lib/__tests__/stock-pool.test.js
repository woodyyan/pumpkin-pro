import { describe, it } from 'node:test'
import assert from 'node:assert/strict'
import { buildStockPoolState } from '../stock-pool.js'

describe('buildStockPoolState', () => {
  it('orders fixed factor lists and maps quote data', () => {
    const state = buildStockPoolState({
      source_trade_date: '2026-08-10',
      market_status: 'trading',
      price_label: '最新价',
      quote_status: 'live',
      lists: [
        {
          factor_key: 'momentum', name: '动量因子指数', status: 'completed', current_constituent_count: 50,
          items: [{ rank: 1, code: '600001', symbol: '600001.SH', name: '样本股', factor_score: 91.5, quote: { last_price: 12.34, change_rate: 0.012, turnover: 3200, status: 'live' } }],
        },
        {
          factor_key: 'value', name: '价值因子指数', status: 'partial', warning_text: '部分日线缺失', current_constituent_count: 49,
          items: [{ rank: 1, code: '600002', symbol: '600002.SH', name: '价值股', factor_score: 98.5, quote: { status: 'unavailable' } }],
        },
      ],
    })

    assert.equal(state.sourceTradeDate, '2026-08-10')
    assert.equal(state.lists[0].factorKey, 'value')
    assert.equal(state.lists[0].statusLabel, '部分数据')
    assert.equal(state.lists[0].items[0].quote.lastPrice, null)
    assert.equal(state.lists[1].factorKey, 'momentum')
    assert.equal(state.lists[1].items[0].quote.lastPrice, 12.34)
    assert.equal(state.lists[1].items[0].quote.changeRate, 0.012)
  })

  it('tolerates missing payloads', () => {
    const state = buildStockPoolState(null)

    assert.deepEqual(state.lists, [])
    assert.equal(state.marketStatus, 'closed')
    assert.equal(state.priceLabel, '最近收盘价')
    assert.equal(state.quoteStatus, 'unavailable')
  })
})
