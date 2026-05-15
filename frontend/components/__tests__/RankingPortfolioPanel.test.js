import { describe, it } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

import {
  buildRankingPortfolioDetailHref,
  buildRankingPortfolioDetailSymbol,
  buildRankingPortfolioChartPoints,
  buildRankingPortfolioChartSeriesData,
  findRankingPortfolioPointByTime,
  formatRankingPortfolioDate,
  formatRankingPortfolioPercent,
  getRankingPortfolioPerformanceClass,
  normalizeRankingPortfolioSeries,
} from '../../lib/ranking-portfolio.js'

const panelSource = readFileSync(new URL('../RankingPortfolioPanel.js', import.meta.url), 'utf8')

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

  it('normalizes series into return percentages and sorted dates', () => {
    const normalized = normalizeRankingPortfolioSeries([
      { date: '2026-05-03', nav: 1.0325, benchmark_nav: 0.991 },
      { date: '2026-05-01T00:00:00.000Z', portfolio_return_pct: 0, benchmark_return_pct: 0 },
      { date: { year: 2026, month: 5, day: 2 }, portfolio_return_pct: 1.5, benchmark_return_pct: 0.5 },
    ])

    assert.deepEqual(normalized.map((item) => item.date), ['2026-05-01', '2026-05-02', '2026-05-03'])
    assert.equal(normalized[1].portfolioReturnPct, 1.5)
    assert.ok(Math.abs(normalized[2].portfolioReturnPct - 3.25) < 1e-9)
    assert.equal(normalized[2].benchmarkReturnPct, -0.9000000000000008)
  })

  it('builds chart series data for lightweight charts', () => {
    const output = buildRankingPortfolioChartSeriesData([
      { date: '2026-05-01', portfolio_return_pct: 0, benchmark_return_pct: 0 },
      { date: '2026-05-02', portfolio_return_pct: 1.25, benchmark_return_pct: 0.75 },
      { date: '2026-05-03', portfolio_return_pct: -0.5, benchmark_return_pct: 0.1, excess_return_pct: -0.6 },
    ])

    assert.equal(output.points.length, 3)
    assert.deepEqual(output.baseline, [
      { time: '2026-05-01', value: 0 },
      { time: '2026-05-02', value: 0 },
      { time: '2026-05-03', value: 0 },
    ])
    assert.deepEqual(output.portfolio[1], { time: '2026-05-02', value: 1.25 })
    assert.deepEqual(output.benchmark[2], { time: '2026-05-03', value: 0.1 })
    assert.equal(output.latest.date, '2026-05-03')
    assert.equal(output.latest.excessReturnPct, -0.6)
  })

  it('finds a point by chart time value', () => {
    const series = [
      { date: '2026-05-01', portfolio_return_pct: 0, benchmark_return_pct: 0 },
      { date: '2026-05-02', portfolio_return_pct: 1.1, benchmark_return_pct: 0.4 },
    ]

    assert.deepEqual(findRankingPortfolioPointByTime(series, '2026-05-02'), {
      date: '2026-05-02',
      portfolioReturnPct: 1.1,
      benchmarkReturnPct: 0.4,
      excessReturnPct: 0.7000000000000001,
    })
    assert.equal(findRankingPortfolioPointByTime(series, { year: 2026, month: 5, day: 3 }), null)
  })

  it('formats ranking portfolio dates safely', () => {
    assert.equal(formatRankingPortfolioDate('2026-05-14'), '2026-05-14')
    assert.equal(formatRankingPortfolioDate('2026-05-14T15:30:00.000Z'), '2026-05-14')
    assert.equal(formatRankingPortfolioDate({ year: 2026, month: 5, day: 9 }), '2026-05-09')
    assert.equal(formatRankingPortfolioDate('invalid-date'), 'invalid-date')
  })

  it('builds detail symbols and hrefs for constituent links', () => {
    assert.equal(buildRankingPortfolioDetailSymbol('700', 'HKEX'), '00700.HK')
    assert.equal(buildRankingPortfolioDetailSymbol('600519', 'SSE'), '600519.SH')
    assert.equal(buildRankingPortfolioDetailSymbol('1', 'SZSE'), '000001.SZ')
    assert.equal(buildRankingPortfolioDetailSymbol('300750', ''), '300750.SZ')
    assert.equal(buildRankingPortfolioDetailHref('600519', 'SSE'), '/live-trading/600519.SH')
    assert.equal(buildRankingPortfolioDetailHref('00700', 'HKEX'), '/live-trading/00700.HK')
    assert.equal(buildRankingPortfolioDetailHref('', 'HKEX'), '')
  })

  it('handles empty series safely', () => {
    const output = buildChartPoints([], 720, 240, 18)
    assert.equal(output.portfolio, '')
    assert.equal(output.benchmark, '')
    assert.deepEqual(buildRankingPortfolioChartSeriesData([]), {
      points: [],
      portfolio: [],
      benchmark: [],
      baseline: [],
      latest: null,
    })
  })
})

describe('RankingPortfolioPanel source contract', () => {
  it('renders constituent rows as detail links and labels weight as 仓位', () => {
    assert.match(panelSource, /buildRankingPortfolioDetailHref/)
    assert.match(panelSource, /target="_blank"/)
    assert.match(panelSource, /rel="noreferrer"/)
    assert.match(panelSource, /仓位/)
  })
})
