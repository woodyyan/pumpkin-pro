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

  it('renders the ranking portfolio panel as the dedicated page content', () => {
    assert.match(pageSource, /RankingPortfolioPanel/)
    assert.match(pageSource, /quadrant\/ranking-portfolio/)
    assert.match(pageSource, /<h1 className="text-xl font-semibold tracking-tight text-foreground">组合跟踪<\/h1>/)
    assert.match(pageSource, /canonical" href="https:\/\/wolongtrader\.top\/portfolio-tracking"/)
    assert.match(pageSource, /组合跟踪页集中展示卧龙AI精选模拟组合的收益曲线、风险指标、当前成分股与最近一次调仓/)
  })
})
