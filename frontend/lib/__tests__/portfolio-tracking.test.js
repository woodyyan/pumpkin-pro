import { describe, it } from 'node:test'
import assert from 'node:assert/strict'

import {
  buildPortfolioTrackingChart,
  buildPortfolioTrackingDetailHref,
  formatPortfolioTrackingCurrency,
  formatPortfolioTrackingPercent,
} from '../portfolio-tracking.js'

describe('portfolio-tracking helpers', () => {
  it('formats percentages with Chinese-market sign convention styles left to UI', () => {
    assert.equal(formatPortfolioTrackingPercent(0.0123), '+1.23%')
    assert.equal(formatPortfolioTrackingPercent(-0.0045), '-0.45%')
  })

  it('formats currencies by market', () => {
    assert.equal(formatPortfolioTrackingCurrency(1000000, 'ASHARE'), '¥1,000,000.00')
    assert.equal(formatPortfolioTrackingCurrency(1000000, 'HKEX'), 'HK$1,000,000.00')
  })

  it('builds stock detail hrefs', () => {
    assert.equal(buildPortfolioTrackingDetailHref('600519', 'SSE'), '/live-trading/600519.SH')
    assert.equal(buildPortfolioTrackingDetailHref('700', 'HKEX'), '/live-trading/00700.HK')
  })

  it('builds chart geometry from daily NAV series', () => {
    const chart = buildPortfolioTrackingChart([
      { trade_date: '2026-06-01', nav: 1.0 },
      { trade_date: '2026-06-02', nav: 1.01 },
      { trade_date: '2026-06-03', nav: 1.02 },
    ])
    assert.ok(chart.path.startsWith('M '))
    assert.equal(chart.points.length, 3)
    assert.ok(Number.isFinite(chart.baselineY))
  })
})
