import { describe, it } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const pageSource = readFileSync(new URL('../../pages/live-trading/[symbol].js', import.meta.url), 'utf8')

describe('live trading portfolio summary section', () => {
  it('links users to portfolio page and keeps the compact summary copy', () => {
    assert.match(pageSource, /去持仓管理查看完整记录/)
    assert.match(pageSource, /查看当前仓位状态，更多记录请前往持仓管理。/)
    assert.match(pageSource, /当前有持仓/)
    assert.match(pageSource, /当前未持有该股票。/)
    assert.match(pageSource, /你可以直接记录一笔买入，或去持仓管理查看完整历史。/)
  })

  it('does not leave removed timeline state references behind', () => {
    assert.doesNotMatch(pageSource, /setPortfolioHistoryExpanded/)
    assert.doesNotMatch(pageSource, /portfolioHistoryExpanded/)
    assert.doesNotMatch(pageSource, /setPortfolioEvents/)
    assert.doesNotMatch(pageSource, /portfolioTimelineLoading/)
  })
  it('removes the old timeline and verbose summary labels', () => {
    assert.doesNotMatch(pageSource, /持仓变化过程/)
    assert.doesNotMatch(pageSource, /持仓成本/)
    assert.doesNotMatch(pageSource, /持仓市值/)
  })
})
