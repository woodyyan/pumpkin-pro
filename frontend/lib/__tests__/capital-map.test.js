import { describe, it } from 'node:test'
import assert from 'node:assert/strict'

import {
  buildCapitalMapUrl,
  buildStockDetailHref,
  changeClassName,
  chartPalette,
  formatBeijingTime,
  formatPercent,
} from '../capital-map.js'

describe('capital map helpers', () => {
  it('uses the confirmed backend route', () => {
    assert.equal(buildCapitalMapUrl(), '/api/capital-map')
  })

  it('formats A-share change using red-up green-down class mapping', () => {
    assert.equal(changeClassName(1.2), 'text-negative')
    assert.equal(changeClassName(-0.7), 'text-positive')
    assert.equal(changeClassName(0), 'text-foreground-muted')
    assert.equal(formatPercent(1.234), '+1.23%')
    assert.equal(formatPercent(-0.7), '-0.70%')
  })

  it('builds stock detail links from Eastmoney market prefixes', () => {
    assert.equal(buildStockDetailHref({ code: '600519', market: 'SH' }), '/live-trading/600519.SH')
    assert.equal(buildStockDetailHref({ code: '000001', market: 'SZ' }), '/live-trading/000001.SZ')
    assert.equal(buildStockDetailHref({ code: '430047', market: 'BJ' }), '/live-trading/430047.BJ')
  })

  it('returns distinct light and dark chart palettes', () => {
    assert.notEqual(chartPalette('light').axis, chartPalette('dark').axis)
    assert.equal(chartPalette('light').red, '#dc2626')
    assert.equal(chartPalette('dark').green, '#22c55e')
  })

  it('formats invalid times safely', () => {
    assert.equal(formatBeijingTime(''), '--')
    assert.equal(formatBeijingTime('not-a-date'), '--')
  })
})
