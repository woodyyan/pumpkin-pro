import { describe, it } from 'node:test'
import assert from 'node:assert/strict'

import {
  buildRankingPortfolioChartPoints,
  formatRankingPortfolioPercent,
  getRankingPortfolioPerformanceClass,
} from '../../lib/ranking-portfolio.js'

function formatPercentValue(value, digits = 2) {
  if (value === null || value === undefined || Number.isNaN(Number(value))) return '--'
  const num = Number(value)
  const sign = num > 0 ? '+' : ''
  return `${sign}${num.toFixed(digits)}%`
}

function getPerformanceClass(value) {
  if (value === null || value === undefined || Number.isNaN(Number(value))) return 'text-white/35'
  return Number(value) >= 0 ? 'text-rose-300' : 'text-emerald-300'
}

function buildChartPoints(series, width, height, padding) {
  if (!Array.isArray(series) || series.length === 0) return { portfolio: '', benchmark: '', baselineY: height / 2 }
  const values = []
  for (const item of series) {
    if (Number.isFinite(Number(item.nav))) values.push(Number(item.nav))
    if (Number.isFinite(Number(item.benchmark_nav))) values.push(Number(item.benchmark_nav))
  }
  const minValue = Math.min(...values, 1)
  const maxValue = Math.max(...values, 1)
  const range = Math.max(maxValue - minValue, 0.001)
  const innerWidth = Math.max(width - padding * 2, 1)
  const innerHeight = Math.max(height - padding * 2, 1)

  const buildPath = (key) => series.map((item, index) => {
    const x = padding + (innerWidth * index) / Math.max(series.length - 1, 1)
    const value = Number(item[key] || 0)
    const y = padding + innerHeight - ((value - minValue) / range) * innerHeight
    return `${index === 0 ? 'M' : 'L'} ${x.toFixed(2)} ${y.toFixed(2)}`
  }).join(' ')

  const baselineY = padding + innerHeight - ((1 - minValue) / range) * innerHeight
  return {
    portfolio: buildPath('nav'),
    benchmark: buildPath('benchmark_nav'),
    baselineY,
  }
}

describe('RankingPortfolioPanel helpers', () => {
  it('formats percent with sign', () => {
    assert.equal(formatPercentValue(3.216), '+3.22%')
    assert.equal(formatPercentValue(-1.2), '-1.20%')
    assert.equal(formatPercentValue(null), '--')
    assert.equal(formatRankingPortfolioPercent(3.216), '+3.22%')
  })

  it('maps performance color correctly', () => {
    assert.equal(getPerformanceClass(0), 'text-rose-300')
    assert.equal(getPerformanceClass(-0.1), 'text-emerald-300')
    assert.equal(getPerformanceClass(undefined), 'text-white/35')
    assert.equal(getRankingPortfolioPerformanceClass(-0.1), 'text-emerald-300')
  })

  it('builds svg paths for series data', () => {
    const output = buildChartPoints([
      { nav: 1, benchmark_nav: 1 },
      { nav: 1.05, benchmark_nav: 1.02 },
      { nav: 1.1, benchmark_nav: 1.03 },
    ], 720, 240, 18)
    assert.ok(output.portfolio.startsWith('M '))
    assert.ok(output.benchmark.includes('L '))
    assert.ok(Number.isFinite(output.baselineY))

    const importedOutput = buildRankingPortfolioChartPoints([
      { nav: 1, benchmark_nav: 1 },
      { nav: 1.05, benchmark_nav: 1.02 },
      { nav: 1.1, benchmark_nav: 1.03 },
    ], 720, 240, 18)
    assert.ok(importedOutput.portfolio.startsWith('M '))
  })

  it('handles empty series safely', () => {
    const output = buildChartPoints([], 720, 240, 18)
    assert.equal(output.portfolio, '')
    assert.equal(output.benchmark, '')
  })
})
