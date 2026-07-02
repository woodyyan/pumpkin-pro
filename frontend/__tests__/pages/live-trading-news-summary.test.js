import { describe, it } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const pageSource = readFileSync(new URL('../../pages/live-trading/[symbol].js', import.meta.url), 'utf8')
const helpersSource = readFileSync(new URL('../../lib/ai-analysis-helpers.js', import.meta.url), 'utf8')
const workspaceSource = readFileSync(new URL('../../components/AIAnalysisWorkspace.js', import.meta.url), 'utf8')

describe('live trading news summary integration', () => {
  it('renders the news summary card near the top of the detail page', () => {
    assert.match(pageSource, /<SymbolNewsSummaryCard/)
    assert.match(pageSource, /summary=\{newsSummary\}/)
    assert.match(pageSource, /<InlineSymbolNewsList/)
    assert.doesNotMatch(pageSource, /onOpen=\{openNewsPanel\}/)
  })

  it('keeps the summary card CTA optional so the news tab can omit 查看全部', () => {
    const cardSource = readFileSync(new URL('../../components/SymbolNewsSummaryCard.js', import.meta.url), 'utf8')
    assert.match(cardSource, /onOpen \? \(/)
    assert.match(cardSource, /查看全部 →/)
  })

  it('loads summary and panel data through dedicated endpoints', () => {
    assert.match(pageSource, /\/news\/summary/)
    assert.match(pageSource, /\?limit=24/)
    assert.match(helpersSource, /\/news\?limit=8/)
    assert.match(pageSource, /NEWS_SUMMARY_REFRESH_MS = 10 \* 60 \* 1000/)
  })

  it('passes news_context into AI analysis payload through the shared helper', () => {
    assert.match(helpersSource, /buildAINewsContext/)
    assert.match(helpersSource, /news_context: newsContext\.payload/)
    assert.match(helpersSource, /export async function fetchAIAnalysisNewsContext/)
  })

  it('surfaces news loading state inside the AI wait panel flow', () => {
    assert.match(pageSource, /setAiNewsContextState\('loading'\)/)
    assert.match(workspaceSource, /新闻上下文暂不可用/)
    assert.match(workspaceSource, /newsState === 'error'/)
  })
})
