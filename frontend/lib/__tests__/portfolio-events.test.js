import { describe, it } from 'node:test'
import assert from 'node:assert/strict'

import {
  buildPortfolioEventPreview,
  buildPortfolioSummaryMetrics,
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
    const form = createPortfolioActionForm('adjust', { avg_cost_price: 12.3 }, { exchange: 'SSE' })
    assert.equal(form.avg_cost_price, '12.3')
    assert.equal(form.exchange, 'ASHARE')
    assert.equal(typeof form.trade_date, 'string')
    assert.equal(form.trade_date.length, 10)
  })

  it('prefills default fee rate for buy and sell actions', () => {
    const buyForm = createPortfolioActionForm('buy', null, { exchange: 'SSE' })
    const sellForm = createPortfolioActionForm('sell', null, { exchange: 'HKEX' })
    assert.equal(buyForm.fee_rate, '0.03')
    assert.equal(sellForm.fee_rate, '0.13')
  })

  it('still uses product defaults when historical profile fee rates are zero', () => {
    const zeroProfile = {
      default_fee_rate_ashare_buy: 0,
      default_fee_rate_ashare_sell: 0,
      default_fee_rate_hk_buy: 0,
      default_fee_rate_hk_sell: 0,
    }
    const buyForm = createPortfolioActionForm('buy', null, { exchange: 'SSE', profile: zeroProfile })
    const sellForm = createPortfolioActionForm('sell', null, { exchange: 'HKEX', profile: zeroProfile })
    assert.equal(buyForm.fee_rate, '0.03')
    assert.equal(sellForm.fee_rate, '0.13')
  })

  it('uses the latest user-configured default fee rates when profile is provided', () => {
    const profile = {
      default_fee_rate_ashare_buy: 0.0002,
      default_fee_rate_ashare_sell: 0.0006,
      default_fee_rate_hk_buy: 0.0011,
      default_fee_rate_hk_sell: 0.0015,
    }
    const buyForm = createPortfolioActionForm('buy', null, { exchange: 'SSE', profile })
    const sellForm = createPortfolioActionForm('sell', null, { exchange: 'HKEX', profile })
    assert.equal(buyForm.fee_rate, '0.02')
    assert.equal(sellForm.fee_rate, '0.15')
  })
})

describe('buildPortfolioEventPreview - buy', () => {
  it('computes weighted average after buy using fee rate', () => {
    const preview = buildPortfolioEventPreview('buy', {
      shares: 100,
      avg_cost_price: 10,
      total_cost_amount: 1000,
      exchange: 'SSE',
    }, {
      quantity: '200',
      price: '13',
      fee_rate: '0.03',
      exchange: 'SSE',
      note: '加仓',
    })
    assert.equal(preview.valid, true)
    assert.equal(preview.nextShares, 300)
    assert.equal(preview.feeAmount, 5)
    assert.equal(preview.feeEstimate.minimumApplied, true)
    assert.equal(preview.nextAvgCostPrice, 12.016666666666667)
    assert.equal(preview.nextTotalCostAmount, 3605)
  })
})

describe('buildPortfolioEventPreview - sell', () => {
  it('keeps average cost unchanged after sell and deducts fee', () => {
    const preview = buildPortfolioEventPreview('sell', {
      shares: 300,
      avg_cost_price: 12,
      total_cost_amount: 3600,
      exchange: 'SSE',
    }, {
      quantity: '100',
      price: '13',
      fee_rate: '0.08',
      exchange: 'SSE',
      note: '减仓',
    })
    assert.equal(preview.valid, true)
    assert.equal(preview.nextShares, 200)
    assert.equal(preview.nextAvgCostPrice, 12)
    assert.equal(preview.nextTotalCostAmount, 2400)
    assert.equal(preview.feeAmount, 5)
    assert.equal(preview.realizedPnlAmount, 95)
  })

  it('rejects selling more than current shares', () => {
    const preview = buildPortfolioEventPreview('sell', {
      shares: 100,
      avg_cost_price: 10,
      total_cost_amount: 1000,
      exchange: 'SSE',
    }, {
      quantity: '120',
      price: '11',
      fee_rate: '0.03',
      exchange: 'SSE',
      note: '超量卖出',
    })
    assert.equal(preview.valid, false)
    assert.match(preview.errors.join(' '), /不能超过当前持仓/)
  })

  it('surfaces minimum commission info for small A股 orders', () => {
    const preview = buildPortfolioEventPreview('sell', {
      shares: 100,
      avg_cost_price: 10,
      total_cost_amount: 1000,
      exchange: 'SSE',
    }, {
      quantity: '10',
      price: '10',
      fee_rate: '0.08',
      exchange: 'SSE',
      note: '小额卖出',
    })
    assert.equal(preview.valid, true)
    assert.equal(preview.feeEstimate.minimumApplied, true)
    assert.equal(preview.feeEstimate.rawFeeAmount, 0.08)
    assert.equal(preview.feeEstimate.finalFeeAmount, 5)
  })
})

describe('buildPortfolioEventPreview - adjust', () => {
  it('requires reason and updates total cost only', () => {
    const preview = buildPortfolioEventPreview('adjust', {
      shares: 200,
      avg_cost_price: 10,
      total_cost_amount: 2000,
      exchange: 'SSE',
    }, {
      avg_cost_price: '9.5',
      note: '补录手续费',
      exchange: 'SSE',
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

describe('buildPortfolioSummaryMetrics', () => {
  it('returns compact metrics for active position with positive pnl', () => {
    const metrics = buildPortfolioSummaryMetrics({
      portfolioData: { shares: 200, avg_cost_price: 10.256 },
      snapshot: { last_price: 12.5 },
      currencySymbol: '¥',
    })
    assert.deepEqual(metrics, [
      { label: '持仓数量', value: '200 股' },
      { label: '买入均价', value: '¥10.256' },
      {
        label: '浮动盈亏',
        value: '+¥448.8（21.88%）',
        accent: 'up',
        emphasis: true,
        marketAccent: true,
        tooltip: '（最新价 - 买入均价）× 持仓数量。红色为盈利，绿色为亏损。',
      },
    ])
  })

  it('returns fallback pnl copy when realtime price is unavailable', () => {
    const metrics = buildPortfolioSummaryMetrics({
      portfolioData: { shares: 0, avg_cost_price: 0 },
      snapshot: null,
      currencySymbol: 'HK$',
    })
    assert.equal(metrics[0].value, '0 股')
    assert.equal(metrics[1].value, '--')
    assert.equal(metrics[2].value, '--')
    assert.equal(metrics[2].accent, 'normal')
  })
})