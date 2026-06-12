import { describe, it } from 'node:test'
import assert from 'node:assert/strict'
import { createRequire } from 'node:module'
import { readFileSync } from 'node:fs'

const requireFromCwd = createRequire(process.cwd() + '/')
const { parse } = requireFromCwd('next/dist/compiled/babel/parser')

const pageCases = [
  { relativePath: '../../pages/ai/analysis.js', title: 'AI分析' },
  { relativePath: '../../pages/ai/picker.js', title: 'AI选股' },
  { relativePath: '../../pages/ai/backtest.js', title: 'AI回测' },
  { relativePath: '../../pages/watchlist.js', title: '自选股' },
]

describe('coming soon placeholder pages', () => {
  for (const pageCase of pageCases) {
    it(`keeps ${pageCase.title} as a minimal placeholder page`, () => {
      const pageSource = readFileSync(new URL(pageCase.relativePath, import.meta.url), 'utf8')

      assert.doesNotThrow(() => parse(pageSource, { sourceType: 'module', plugins: ['jsx'] }))
      assert.match(pageSource, /ComingSoonPage/)
      assert.match(pageSource, new RegExp(`title=\"${pageCase.title}\"`))
    })
  }

  it('renders the shared placeholder copy', () => {
    const componentSource = readFileSync(new URL('../../components/ComingSoonPage.js', import.meta.url), 'utf8')

    assert.doesNotThrow(() => parse(componentSource, { sourceType: 'module', plugins: ['jsx'] }))
    assert.match(componentSource, /敬请期待/)
  })

  it('renders quadrant as a real page instead of a placeholder', () => {
    const pageSource = readFileSync(new URL('../../pages/quadrant.js', import.meta.url), 'utf8')

    assert.doesNotThrow(() => parse(pageSource, { sourceType: 'module', plugins: ['jsx'] }))
    assert.doesNotMatch(pageSource, /ComingSoonPage/)
    assert.match(pageSource, /QuadrantOverviewSection/)
    assert.match(pageSource, /RankingOverviewSection/)
    assert.match(pageSource, /canonical" href="https:\/\/wolongtrader\.top\/quadrant"/)
  })

  it('renders portfolio tracking as a real page instead of a placeholder', () => {
    const pageSource = readFileSync(new URL('../../pages/portfolio-tracking.js', import.meta.url), 'utf8')

    assert.doesNotThrow(() => parse(pageSource, { sourceType: 'module', plugins: ['jsx'] }))
    assert.doesNotMatch(pageSource, /ComingSoonPage/)
    assert.match(pageSource, /RankingPortfolioPanel/)
    assert.match(pageSource, /canonical" href="https:\/\/wolongtrader\.top\/portfolio-tracking"/)
  })
})
