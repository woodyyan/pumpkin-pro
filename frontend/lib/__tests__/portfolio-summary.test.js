import { describe, it } from 'node:test'
import assert from 'node:assert/strict'

import { buildPortfolioOverviewBlocks } from '../portfolio-summary.js'

describe('buildPortfolioOverviewBlocks', () => {
  it('returns mixed-market blocks as-is when dashboard contains multiple markets', () => {
    const blocks = buildPortfolioOverviewBlocks({
      mixed_currency: true,
      amounts_by_market: [
        { scope: 'ASHARE', scope_label: 'A股', market_value_amount: 120000 },
        { scope: 'HKEX', scope_label: '港股', market_value_amount: 90000 },
      ],
    })

    assert.equal(blocks.length, 2)
    assert.equal(blocks[0].scope, 'ASHARE')
    assert.equal(blocks[1].scope, 'HKEX')
  })

  it('builds a unified A-share block for a single-market scoped summary', () => {
    const blocks = buildPortfolioOverviewBlocks({
      scope: 'ASHARE',
      mixed_currency: false,
      position_count: 3,
      max_position_weight_ratio: 0.42,
      amounts: {
        currency_code: 'CNY',
        currency_symbol: '¥',
        market_value_amount: 250000,
        total_cost_amount: 230000,
        unrealized_pnl_amount: 12000,
        realized_pnl_amount: 8000,
        total_pnl_amount: 20000,
        today_pnl_amount: 1500,
      },
    })

    assert.equal(blocks.length, 1)
    assert.deepEqual(blocks[0], {
      scope: 'ASHARE',
      scope_label: 'A股',
      currency_code: 'CNY',
      currency_symbol: '¥',
      market_value_amount: 250000,
      total_cost_amount: 230000,
      unrealized_pnl_amount: 12000,
      realized_pnl_amount: 8000,
      total_pnl_amount: 20000,
      today_pnl_amount: 1500,
      position_count: 3,
      max_position_weight_ratio: 0.42,
    })
  })

  it('infers the market from currency when scope is ALL but only one market exists', () => {
    const blocks = buildPortfolioOverviewBlocks({
      scope: 'ALL',
      mixed_currency: false,
      position_count: 1,
      max_position_weight_ratio: 0.68,
      amounts: {
        currency_code: 'HKD',
        market_value_amount: 88888,
        total_cost_amount: 80000,
        unrealized_pnl_amount: 5000,
        realized_pnl_amount: 1200,
        total_pnl_amount: 6200,
        today_pnl_amount: -320,
      },
    })

    assert.equal(blocks.length, 1)
    assert.equal(blocks[0].scope, 'HKEX')
    assert.equal(blocks[0].scope_label, '港股')
    assert.equal(blocks[0].currency_symbol, 'HK$')
    assert.equal(blocks[0].position_count, 1)
    assert.equal(blocks[0].max_position_weight_ratio, 0.68)
  })

  it('returns an empty list when summary data is unavailable', () => {
    assert.deepEqual(buildPortfolioOverviewBlocks(null), [])
    assert.deepEqual(buildPortfolioOverviewBlocks({ mixed_currency: false, amounts: null }), [])
  })
})
