import { describe, it } from 'node:test'
import assert from 'node:assert/strict'

import {
  buildPortfolioEventPreview,
  createPortfolioActionForm,
  formatPortfolioEventHeadline,
  formatPortfolioEventSubline,
  getPortfolioEventAccent,
  isPortfolioPositionActive,
} from '../portfolio-events.js'

describe('isPortfolioPositionActive', () => {
  it('returns true only when shares > 0', () => {
    assert.equal(isPortfolioPositionActive({ shares: 100 }), true)
    assert.equal(isPortfolioPositionActive({ shares: 0 }), false)
    assert.equal(isPortfolioPositionActive(null), false)
  })
})

describe('createPortfolioActionForm', () => {
  it('prefills avg cost for adjust action', () => {
    const form = createPortfolioActionForm('adjust', { avg_cost_price: 12.3 })
    assert.equal(form.avg_cost_price, '12.3')
    assert.equal(typeof form.trade_date, 'string')
    assert.equal(form.trade_date.length, 10)
  })
})

describe('buildPortfolioEventPreview - buy', () => {
  it('computes weighted average after buy', () => {
    const preview = buildPortfolioEventPreview('buy', {
      shares: 100,
      avg_cost_price: 10,
      total_cost_amount: 1000,
    }, {
      quantity: '200',
      price: '13',
      fee_amount: '0',
      note: '加仓',
    })
    assert.equal(preview.valid, true)
    assert.equal(preview.nextShares, 300)
    assert.equal(preview.nextAvgCostPrice, 12)
    assert.equal(preview.nextTotalCostAmount, 3600)
  })
})

describe('buildPortfolioEventPreview - sell', () => {
  it('keeps average cost unchanged after sell', () => {
    const preview = buildPortfolioEventPreview('sell', {
      shares: 300,
      avg_cost_price: 12,
      total_cost_amount: 3600,
    }, {
      quantity: '100',
      price: '13',
      fee_amount: '0',
      note: '减仓',
    })
    assert.equal(preview.valid, true)
    assert.equal(preview.nextShares, 200)
    assert.equal(preview.nextAvgCostPrice, 12)
    assert.equal(preview.nextTotalCostAmount, 2400)
    assert.equal(preview.realizedPnlAmount, 100)
  })

  it('rejects selling more than current shares', () => {
    const preview = buildPortfolioEventPreview('sell', {
      shares: 100,
      avg_cost_price: 10,
      total_cost_amount: 1000,
    }, {
      quantity: '120',
      price: '11',
      fee_amount: '0',
      note: '超量卖出',
    })
    assert.equal(preview.valid, false)
    assert.match(preview.errors.join(' '), /不能超过当前持仓/)
  })
})

describe('buildPortfolioEventPreview - adjust', () => {
  it('requires reason and updates total cost only', () => {
    const preview = buildPortfolioEventPreview('adjust', {
      shares: 200,
      avg_cost_price: 10,
      total_cost_amount: 2000,
    }, {
      avg_cost_price: '9.5',
      note: '补录手续费',
    })
    assert.equal(preview.valid, true)
    assert.equal(preview.nextShares, 200)
    assert.equal(preview.nextAvgCostPrice, 9.5)
    assert.equal(preview.nextTotalCostAmount, 1900)
  })
})

describe('portfolio event copy helpers', () => {
  it('formats headline and subline for buy events', () => {
    const event = {
      event_type: 'buy',
      quantity: 200,
      price: 13,
      before_shares: 100,
      after_shares: 300,
      after_avg_cost_price: 12,
    }
    assert.equal(formatPortfolioEventHeadline(event, '600036').includes('买入 200 股'), true)
    assert.equal(formatPortfolioEventSubline(event, '600036').includes('100 股 → 300 股'), true)
    assert.equal(getPortfolioEventAccent(event), 'text-rose-300')
  })

  it('formats sell and adjust events', () => {
    const sellEvent = {
      event_type: 'sell',
      quantity: 100,
      price: 13.1,
      before_shares: 300,
      after_shares: 200,
      realized_pnl_amount: 110,
    }
    const adjustEvent = {
      event_type: 'adjust_avg_cost',
      before_avg_cost_price: 12.6,
      after_avg_cost_price: 12.1,
      after_shares: 200,
    }
    assert.equal(formatPortfolioEventHeadline(sellEvent, '600036').includes('卖出 100 股'), true)
    assert.equal(formatPortfolioEventSubline(sellEvent, '600036').includes('已实现收益'), true)
    assert.equal(formatPortfolioEventHeadline(adjustEvent, '600036'), '手动调整均价')
    assert.equal(formatPortfolioEventSubline(adjustEvent, '600036').includes('持仓 200 股未变'), true)
  })
})
