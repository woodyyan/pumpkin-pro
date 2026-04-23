import { describe, it } from 'node:test'
import assert from 'node:assert/strict'

import {
  calculatePortfolioFeeEstimate,
  describeFeeRate,
  describePortfolioFeeEstimate,
  formatFeeRatePercent,
  formatPortfolioFeeAmount,
  getPortfolioDefaultFeeRate,
  getPortfolioMinimumCommission,
  getPortfolioSystemDefaultFeeRate,
  normalizePortfolioExchange,
  parseFeeRatePercentInput,
} from '../portfolio-fee.js'

describe('normalizePortfolioExchange', () => {
  it('normalizes A股 and 港股 aliases', () => {
    assert.equal(normalizePortfolioExchange('SSE'), 'ASHARE')
    assert.equal(normalizePortfolioExchange('SZSE'), 'ASHARE')
    assert.equal(normalizePortfolioExchange('HKEX'), 'HKEX')
    assert.equal(normalizePortfolioExchange(''), 'ASHARE')
  })
})

describe('getPortfolioSystemDefaultFeeRate', () => {
  it('returns product defaults for A股 and 港股 buy/sell', () => {
    assert.equal(getPortfolioSystemDefaultFeeRate({ exchange: 'SSE', action: 'buy' }), 0.0003)
    assert.equal(getPortfolioSystemDefaultFeeRate({ exchange: 'SZSE', action: 'sell' }), 0.0008)
    assert.equal(getPortfolioSystemDefaultFeeRate({ exchange: 'HKEX', action: 'buy' }), 0.0013)
    assert.equal(getPortfolioSystemDefaultFeeRate({ exchange: 'HKEX', action: 'sell' }), 0.0013)
  })
})

describe('getPortfolioDefaultFeeRate', () => {
  it('falls back to system defaults when profile is missing', () => {
    assert.equal(getPortfolioDefaultFeeRate({ exchange: 'SSE', action: 'buy' }), 0.0003)
    assert.equal(getPortfolioDefaultFeeRate({ exchange: 'SZSE', action: 'sell' }), 0.0008)
    assert.equal(getPortfolioDefaultFeeRate({ exchange: 'HKEX', action: 'buy' }), 0.0013)
    assert.equal(getPortfolioDefaultFeeRate({ exchange: 'HKEX', action: 'sell' }), 0.0013)
  })

  it('falls back to system defaults when historical profile values are zero', () => {
    const profile = {
      default_fee_rate_ashare_buy: 0,
      default_fee_rate_ashare_sell: 0,
      default_fee_rate_hk_buy: 0,
      default_fee_rate_hk_sell: 0,
    }
    assert.equal(getPortfolioDefaultFeeRate({ exchange: 'SSE', action: 'buy', profile }), 0.0003)
    assert.equal(getPortfolioDefaultFeeRate({ exchange: 'SZSE', action: 'sell', profile }), 0.0008)
    assert.equal(getPortfolioDefaultFeeRate({ exchange: 'HKEX', action: 'buy', profile }), 0.0013)
    assert.equal(getPortfolioDefaultFeeRate({ exchange: 'HKEX', action: 'sell', profile }), 0.0013)
  })

  it('prefers user configured values', () => {
    const profile = {
      default_fee_rate_ashare_buy: 0.0005,
      default_fee_rate_ashare_sell: 0.0009,
      default_fee_rate_hk_buy: 0.0011,
      default_fee_rate_hk_sell: 0.0012,
    }
    assert.equal(getPortfolioDefaultFeeRate({ exchange: 'SSE', action: 'buy', profile }), 0.0005)
    assert.equal(getPortfolioDefaultFeeRate({ exchange: 'SSE', action: 'sell', profile }), 0.0009)
    assert.equal(getPortfolioDefaultFeeRate({ exchange: 'HKEX', action: 'buy', profile }), 0.0011)
    assert.equal(getPortfolioDefaultFeeRate({ exchange: 'HKEX', action: 'sell', profile }), 0.0012)
  })
})

describe('getPortfolioMinimumCommission', () => {
  it('returns fixed 5元 minimum commission only for A股 buy and sell', () => {
    assert.equal(getPortfolioMinimumCommission({ exchange: 'SSE', action: 'buy' }), 5)
    assert.equal(getPortfolioMinimumCommission({ exchange: 'SZSE', action: 'sell' }), 5)
    assert.equal(getPortfolioMinimumCommission({ exchange: 'HKEX', action: 'buy' }), 0)
    assert.equal(getPortfolioMinimumCommission({ exchange: 'SSE', action: 'adjust' }), 0)
  })
})

describe('calculatePortfolioFeeEstimate', () => {
  it('computes normal fee amount and rounds to 2 decimals', () => {
    const result = calculatePortfolioFeeEstimate({
      exchange: 'SSE',
      action: 'buy',
      quantity: 1000,
      price: 12.34,
      feeRate: 0.0003,
    })
    assert.deepEqual(result, {
      rawFeeAmount: 3.7,
      finalFeeAmount: 5,
      minimumFeeAmount: 5,
      minimumApplied: true,
    })
  })

  it('applies minimum commission for small A股 buy orders', () => {
    const result = calculatePortfolioFeeEstimate({
      exchange: 'ASHARE',
      action: 'buy',
      quantity: 100,
      price: 10,
      feeRate: 0.0003,
    })
    assert.equal(result.rawFeeAmount, 0.3)
    assert.equal(result.finalFeeAmount, 5)
    assert.equal(result.minimumApplied, true)
  })

  it('applies minimum commission for small A股 sell orders', () => {
    const result = calculatePortfolioFeeEstimate({
      exchange: 'SZSE',
      action: 'sell',
      quantity: 100,
      price: 8,
      feeRate: 0.0008,
    })
    assert.equal(result.rawFeeAmount, 0.64)
    assert.equal(result.finalFeeAmount, 5)
    assert.equal(result.minimumApplied, true)
  })

  it('does not apply minimum commission for 港股 orders', () => {
    const result = calculatePortfolioFeeEstimate({
      exchange: 'HKEX',
      action: 'buy',
      quantity: 100,
      price: 20,
      feeRate: 0.0013,
    })
    assert.equal(result.rawFeeAmount, 2.6)
    assert.equal(result.finalFeeAmount, 2.6)
    assert.equal(result.minimumApplied, false)
  })

  it('does not force minimum commission when feeRate is zero', () => {
    const result = calculatePortfolioFeeEstimate({
      exchange: 'SSE',
      action: 'buy',
      quantity: 100,
      price: 20,
      feeRate: 0,
    })
    assert.equal(result.rawFeeAmount, 0)
    assert.equal(result.finalFeeAmount, 0)
    assert.equal(result.minimumApplied, false)
  })
})

describe('fee rate formatting helpers', () => {
  it('formats and parses percentage input correctly', () => {
    assert.equal(formatFeeRatePercent(0.0003), '0.03')
    assert.equal(formatFeeRatePercent(0.0013), '0.13')
    assert.equal(parseFeeRatePercentInput('0.03'), 0.0003)
    assert.equal(parseFeeRatePercentInput('0.13'), 0.0013)
  })

  it('describes fee rate for helper text', () => {
    assert.equal(describeFeeRate(0.0003), '约万3')
    assert.equal(describeFeeRate(0.0013), '约 0.13%')
  })
})

describe('fee estimate copy helpers', () => {
  it('formats money for CNY and HKD', () => {
    assert.equal(formatPortfolioFeeAmount(5, 'SSE'), '¥5.00')
    assert.equal(formatPortfolioFeeAmount(2.6, 'HKEX'), 'HK$2.60')
  })

  it('builds copy for standard fee estimate', () => {
    const copy = describePortfolioFeeEstimate({
      exchange: 'HKEX',
      feeEstimate: { rawFeeAmount: 2.6, finalFeeAmount: 2.6, minimumFeeAmount: 0, minimumApplied: false },
    })
    assert.equal(copy, '按本次成交金额估算手续费：HK$2.60')
  })

  it('builds copy when minimum commission is applied', () => {
    const copy = describePortfolioFeeEstimate({
      exchange: 'SSE',
      feeEstimate: { rawFeeAmount: 0.3, finalFeeAmount: 5, minimumFeeAmount: 5, minimumApplied: true },
    })
    assert.equal(copy, '按费率估算约 ¥0.30，低于最低佣金 ¥5.00，本次按 ¥5.00 结算。')
  })
})
