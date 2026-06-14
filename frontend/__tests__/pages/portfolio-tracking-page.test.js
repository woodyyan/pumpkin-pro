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
    assert.match(pageSource, /这里会展示卧龙量化因子选股出来的结果，并持续跟踪他们的收益，方便大家参考。/)
  })
})
