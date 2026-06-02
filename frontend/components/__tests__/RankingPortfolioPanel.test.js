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
  formatRankingPortfolioReferencePrice,
  formatRankingPortfolioPercent,
  formatRankingPortfolioWeight,
  formatRankingPortfolioWeightChange,
  getRankingPortfolioRebalanceActionLabel,
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
  if (value === null || value === undefined || Number.isNaN(Number(value))) return 'text-foreground-dim'
  return Number(value) >= 0 ? 'text-negative' : 'text-positive'
}

function buildChartPoints(series, width, height, padding) {
  if (!Array.isArray(series) || series.length === 0) return { portfolio: '', baselineY: height / 2 }
  const values = []
  for (const item of series) {
    if (Number.isFinite(Number(item.nav))) values.push(Number(item.nav))
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
    assert.equal(getPerformanceClass(0), 'text-negative')
    assert.equal(getPerformanceClass(-0.1), 'text-positive')
    assert.equal(getPerformanceClass(undefined), 'text-foreground-dim')
    assert.equal(getRankingPortfolioPerformanceClass(-0.1), 'text-positive')
  })

  it('builds svg paths for series data', () => {
    const output = buildChartPoints([
      { nav: 1 },
      { nav: 1.05 },
      { nav: 1.1 },
    ], 720, 240, 18)
    assert.ok(output.portfolio.startsWith('M '))
    assert.ok(Number.isFinite(output.baselineY))

    const importedOutput = buildRankingPortfolioChartPoints([
      { nav: 1 },
      { nav: 1.05 },
      { nav: 1.1 },
    ], 720, 240, 18)
    assert.ok(importedOutput.portfolio.startsWith('M '))
  })

  it('normalizes series into return percentages and sorted dates', () => {
    const normalized = normalizeRankingPortfolioSeries([
      { date: '2026-05-03', nav: 1.0325 },
      { date: '2026-05-01T00:00:00.000Z', portfolio_return_pct: 0, daily_portfolio_return_pct: 0 },
      { date: { year: 2026, month: 5, day: 2 }, portfolio_return_pct: 1.5, drawdown_pct: -0.2 },
    ])

    assert.deepEqual(normalized.map((item) => item.date), ['2026-05-01', '2026-05-02', '2026-05-03'])
    assert.equal(normalized[1].portfolioReturnPct, 1.5)
    assert.equal(normalized[1].drawdownPct, -0.2)
    assert.ok(Math.abs(normalized[2].portfolioReturnPct - 3.25) < 1e-9)
    assert.equal(normalized[2].drawdownPct, 0)
  })

  it('builds chart series data for lightweight charts', () => {
    const output = buildRankingPortfolioChartSeriesData([
      { date: '2026-05-01', portfolio_return_pct: 0, daily_portfolio_return_pct: 0 },
      { date: '2026-05-02', portfolio_return_pct: 1.25, daily_portfolio_return_pct: 1.25 },
      { date: '2026-05-03', portfolio_return_pct: -0.5, daily_portfolio_return_pct: -1.75 },
    ])

    assert.equal(output.points.length, 3)
    assert.deepEqual(output.baseline, [
      { time: '2026-05-01', value: 0 },
      { time: '2026-05-02', value: 0 },
      { time: '2026-05-03', value: 0 },
    ])
    assert.deepEqual(output.portfolio[1], { time: '2026-05-02', value: 1.25 })
    assert.equal(output.latest.date, '2026-05-03')
    assert.equal(output.latest.drawdownPct, -1.75)
  })

  it('finds a point by chart time value', () => {
    const series = [
      { date: '2026-05-01', portfolio_return_pct: 0, daily_portfolio_return_pct: 0 },
      { date: '2026-05-02', portfolio_return_pct: 1.1, daily_portfolio_return_pct: 1.1 },
    ]

    assert.deepEqual(findRankingPortfolioPointByTime(series, '2026-05-02'), {
      date: '2026-05-02',
      portfolioReturnPct: 1.1,
      dailyPortfolioReturnPct: 1.1,
      drawdownPct: 0,
    })
    assert.equal(findRankingPortfolioPointByTime(series, { year: 2026, month: 5, day: 3 }), null)
  })

  it('formats ranking portfolio dates safely', () => {
    assert.equal(formatRankingPortfolioDate('2026-05-14'), '2026-05-14')
    assert.equal(formatRankingPortfolioDate('2026-05-14T15:30:00.000Z'), '2026-05-14')
    assert.equal(formatRankingPortfolioDate({ year: 2026, month: 5, day: 9 }), '2026-05-09')
    assert.equal(formatRankingPortfolioDate('invalid-date'), 'invalid-date')
  })

  it('formats rebalance labels, weights and reference prices', () => {
    assert.equal(getRankingPortfolioRebalanceActionLabel('sell'), '卖出')
    assert.equal(getRankingPortfolioRebalanceActionLabel('buy'), '买入')
    assert.equal(formatRankingPortfolioWeight(0.25), '25%')
    assert.equal(formatRankingPortfolioWeightChange(0, 0.25), '0% -> 25%')
    assert.equal(formatRankingPortfolioReferencePrice(43.9912, 'SSE'), '¥43.99')
    assert.equal(formatRankingPortfolioReferencePrice(388.6, 'HKEX'), 'HK$388.60')
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
    assert.deepEqual(buildRankingPortfolioChartSeriesData([]), {
      points: [],
      portfolio: [],
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

  it('renders compact exchange and variant toggles with B-combo metadata', () => {
    assert.match(panelSource, /A \/ B 双组合/)
    assert.match(panelSource, /跟踪卧龙AI精选 A、B 两套组合的收盘价模拟表现，核心看累计收益、回撤、波动与日胜率。/)
    assert.match(panelSource, /selectedExchange/)
    assert.match(panelSource, /selectedVariant/)
    assert.match(panelSource, /模拟组合A/)
    assert.match(panelSource, /模拟组合B/)
    assert.match(panelSource, /剔除科创板/)
    assert.match(panelSource, /\$\{windowText\} 连续上榜优先/)
    assert.match(panelSource, /榜单第\$\{item\?\.source_rank \|\| '--'\}名/)
    assert.match(panelSource, /连续\$\{item\?\.consecutive_days \|\| '--'\}日/)
    assert.match(panelSource, /\$\{closeDateLabel\}，\$\{effectiveDate\} 开盘生效/)
    assert.match(panelSource, /较买入价/)
    assert.doesNotMatch(panelSource, /每日收盘后取卧龙AI精选排行榜A、B两种规则 4 只股票/)
    assert.doesNotMatch(panelSource, /数据日期：/)
  })

  it('imports every React hook it uses', () => {
    assert.match(panelSource, /import \{[^}]*useEffect[^}]*\} from 'react'/)
    assert.match(panelSource, /useEffect\(/)
  })

  it('imports rebalance formatter helpers used in disclosure rows', () => {
    assert.match(panelSource, /import \{[^}]*formatRankingPortfolioReferencePrice[^}]*formatRankingPortfolioWeightChange[^}]*\} from '\.\.\/lib\/ranking-portfolio'/)
    assert.match(panelSource, /formatRankingPortfolioWeightChange\(item\?\.from_weight, item\?\.to_weight, 0\)/)
  })

  it('keeps latest rebalance collapsed behind a disclosure button', () => {
    assert.match(panelSource, /最近一次调仓/)
    assert.match(panelSource, /<details/)
    assert.match(panelSource, /changeCount}项/)
    assert.match(panelSource, /参考成本价/)
    assert.match(panelSource, /本次未发生调仓/)
  })
})
