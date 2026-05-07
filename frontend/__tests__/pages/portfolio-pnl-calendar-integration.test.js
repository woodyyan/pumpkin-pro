import { describe, it } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const pageSource = readFileSync(new URL('../../pages/portfolio.js', import.meta.url), 'utf8')

describe('portfolio pnl calendar integration', () => {
  it('imports and renders the PortfolioPnlCalendar component', () => {
    assert.match(pageSource, /import PortfolioPnlCalendar from '\.\.\/components\/PortfolioPnlCalendar'/)
    assert.match(pageSource, /<PortfolioPnlCalendar/)
  })

  it('places the core dashboard before attribution and removes the old charts section', () => {
    const coreDashboardIndex = pageSource.indexOf('<PortfolioCoreDashboardSection')
    const attributionIndex = pageSource.indexOf('<PortfolioAttributionSection')
    assert.ok(coreDashboardIndex > -1, 'PortfolioCoreDashboardSection not found')
    assert.ok(attributionIndex > -1, 'PortfolioAttributionSection not found')
    assert.ok(coreDashboardIndex < attributionIndex, 'core dashboard should render before attribution')
    assert.doesNotMatch(pageSource, /<PortfolioChartsSection/)
    assert.doesNotMatch(pageSource, /function PortfolioChartsSection/)
  })

  it('keeps mobile DOM order as positions, allocation, equity curve, then pnl calendar', () => {
    const coreDashboardStart = pageSource.indexOf('function PortfolioCoreDashboardSection')
    const coreDashboardEnd = pageSource.indexOf('// ── 风险仪表盘 ──')
    const coreDashboardSource = coreDashboardStart >= 0 && coreDashboardEnd > coreDashboardStart
      ? pageSource.slice(coreDashboardStart, coreDashboardEnd)
      : ''
    const positionsIndex = coreDashboardSource.indexOf('<PositionDetailPanel')
    const allocationIndex = coreDashboardSource.indexOf('<AllocationPie')
    const curveIndex = coreDashboardSource.indexOf('<EquityCurveSection')
    const calendarIndex = coreDashboardSource.indexOf('<PortfolioPnlCalendar')
    assert.ok(coreDashboardSource, 'PortfolioCoreDashboardSection source not found')
    assert.ok(positionsIndex > -1, 'PositionDetailPanel not found')
    assert.ok(allocationIndex > -1, 'AllocationPie not found')
    assert.ok(curveIndex > -1, 'EquityCurveSection not found')
    assert.ok(calendarIndex > -1, 'PortfolioPnlCalendar not found')
    assert.ok(positionsIndex < allocationIndex, 'positions should render before allocation')
    assert.ok(allocationIndex < curveIndex, 'allocation should render before equity curve')
    assert.ok(curveIndex < calendarIndex, 'equity curve should render before pnl calendar')
  })

  it('keeps positions and allocation in the first desktop row with an internal position scroll area', () => {
    const detailStart = pageSource.indexOf('function PositionDetailPanel')
    const detailEnd = pageSource.indexOf('function TradeMetric')
    const detailSource = detailStart >= 0 && detailEnd > detailStart
      ? pageSource.slice(detailStart, detailEnd)
      : ''
    const coreDashboardStart = pageSource.indexOf('function PortfolioCoreDashboardSection')
    const coreDashboardEnd = pageSource.indexOf('// ── 风险仪表盘 ──')
    const coreDashboardSource = coreDashboardStart >= 0 && coreDashboardEnd > coreDashboardStart
      ? pageSource.slice(coreDashboardStart, coreDashboardEnd)
      : ''
    assert.match(coreDashboardSource, /grid items-stretch gap-4 xl:grid-cols-\[1\.45fr_1fr\]/)
    assert.match(detailSource, /flex h-full min-h-0 flex-col/)
    assert.match(detailSource, /min-h-0 flex-1 xl:overflow-y-auto/)
    assert.match(detailSource, /custom-scrollbar/)
  })

  it('defaults display metric to amount and refreshes after dashboard load', () => {
    assert.match(pageSource, /useState\('amount'\)/)
    assert.match(pageSource, /setPnlCalendarRefreshVersion\(\(prev\) => prev \+ 1\)/)
  })

  it('keeps metric switching client-side', () => {
    assert.match(pageSource, /onCalendarDisplayMetricChange=\{setCalendarDisplayMetric\}/)
    assert.match(pageSource, /onDisplayMetricChange=\{onCalendarDisplayMetricChange\}/)
    assert.doesNotMatch(pageSource, /calendarDisplayMetric[\s\S]{0,120}fetchPortfolioPnlCalendar/)
  })
})
