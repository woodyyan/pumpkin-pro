import { describe, it } from 'node:test'
import assert from 'node:assert/strict'
import { createRequire } from 'node:module'
import { readFileSync } from 'node:fs'

const requireFromCwd = createRequire(process.cwd() + '/')
const { parse } = requireFromCwd('next/dist/compiled/babel/parser')
const pageSource = readFileSync(new URL('../../pages/portfolio-tracking.js', import.meta.url), 'utf8')

describe('portfolio tracking page', () => {
  it('parses as valid JSX', () => {
    assert.doesNotThrow(() => {
      parse(pageSource, {
        sourceType: 'module',
        plugins: ['jsx'],
      })
    })
  })

  it('loads the new portfolio tracking overview and detail endpoints', () => {
    assert.match(pageSource, /PortfolioTrackingDashboard/)
    assert.match(pageSource, /\/api\/portfolio-tracking\/overview/)
    assert.match(pageSource, /\/api\/portfolio-tracking\/\$\{encodeURIComponent\(portfolioId\)\}\/daily/)
    assert.match(pageSource, /URLSearchParams/)
    assert.match(pageSource, /新的模拟组合事实表口径/)
    assert.match(pageSource, /canonical" href="https:\/\/wolongtrader\.top\/portfolio-tracking"/)
  })
})
