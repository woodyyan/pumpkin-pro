import { describe, it } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const pageSource = readFileSync(new URL('../../pages/live-trading/[symbol].js', import.meta.url), 'utf8')
const helpersSource = readFileSync(new URL('../../lib/ai-analysis-helpers.js', import.meta.url), 'utf8')
const workspaceSource = readFileSync(new URL('../../components/AIAnalysisWorkspace.js', import.meta.url), 'utf8')

describe('live trading news tab removal', () => {
  it('no longer renders the news summary card or inline news list', () => {
    assert.doesNotMatch(pageSource, /<SymbolNewsSummaryCard/)
    assert.doesNotMatch(pageSource, /<InlineSymbolNewsList/)
    assert.doesNotMatch(pageSource, /STOCK_DETAIL_TAB_KEYS\.NEWS/)
    assert.doesNotMatch(pageSource, /loadNewsSummary|loadNewsItems/)
  })

  it('passes news_context into AI analysis payload through the shared helper', () => {
    assert.match(helpersSource, /buildAINewsContext/)
    assert.match(helpersSource, /news_context: newsContext\.payload/)
    assert.match(helpersSource, /export async function fetchAIAnalysisNewsContext/)
    assert.match(helpersSource, /\/news\?limit=8/)
  })

  it('surfaces news loading state inside the AI wait panel flow', () => {
    assert.match(pageSource, /setAiNewsContextState\('loading'\)/)
    assert.match(workspaceSource, /新闻上下文暂不可用/)
    assert.match(workspaceSource, /newsState === 'error'/)
  })
})
