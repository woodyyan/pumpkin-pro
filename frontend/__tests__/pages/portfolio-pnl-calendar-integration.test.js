import { describe, it } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const pageSource = readFileSync(new URL('../../pages/portfolio.js', import.meta.url), 'utf8')

describe('portfolio pnl calendar integration', () => {
  it('imports and renders the PortfolioPnlCalendar component', () => {
    assert.match(pageSource, /import PortfolioPnlCalendar from '\.\.\/components\/PortfolioPnlCalendar'/)
    assert.match(pageSource, /<PortfolioPnlCalendar/)
  })

  it('places positions between charts and pnl calendar, and keeps pnl calendar before attribution', () => {
    const chartsIndex = pageSource.indexOf('<PortfolioChartsSection')
    const positionsIndex = pageSource.indexOf('<PositionTable positions={data.positions}')
    const calendarIndex = pageSource.indexOf('<PortfolioPnlCalendar')
    const attributionIndex = pageSource.indexOf('<PortfolioAttributionSection')
    assert.ok(chartsIndex > -1, 'PortfolioChartsSection not found')
    assert.ok(positionsIndex > -1, 'PositionTable not found')
    assert.ok(calendarIndex > -1, 'PortfolioPnlCalendar not found')
    assert.ok(attributionIndex > -1, 'PortfolioAttributionSection not found')
    assert.ok(chartsIndex < positionsIndex, 'positions should render after charts')
    assert.ok(positionsIndex < calendarIndex, 'positions should render before calendar')
    assert.ok(calendarIndex < attributionIndex, 'calendar should render before attribution')
  })

  it('defaults display metric to amount and refreshes after dashboard load', () => {
    assert.match(pageSource, /useState\('amount'\)/)
    assert.match(pageSource, /setPnlCalendarRefreshVersion\(\(prev\) => prev \+ 1\)/)
  })

  it('keeps metric switching client-side', () => {
    assert.match(pageSource, /onDisplayMetricChange=\{setCalendarDisplayMetric\}/)
    assert.doesNotMatch(pageSource, /calendarDisplayMetric[\s\S]{0,120}fetchPortfolioPnlCalendar/)
  })
})
