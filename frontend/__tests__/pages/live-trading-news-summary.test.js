import { describe, it } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const pageSource = readFileSync(new URL('../../pages/live-trading/[symbol].js', import.meta.url), 'utf8')

describe('live trading news summary integration', () => {
  it('renders the news summary card near the top of the detail page', () => {
    assert.match(pageSource, /<SymbolNewsSummaryCard/)
    assert.match(pageSource, /summary=\{newsSummary\}/)
    assert.match(pageSource, /onOpen=\{openNewsPanel\}/)
  })

  it('loads summary and panel data through dedicated endpoints', () => {
    assert.match(pageSource, /\/news\/summary/)
    assert.match(pageSource, /\/news\?limit=8/)
    assert.match(pageSource, /NEWS_SUMMARY_REFRESH_MS = 10 \* 60 \* 1000/)
  })

  it('passes news_context into AI analysis payload', () => {
    assert.match(pageSource, /buildAINewsContext/)
    assert.match(pageSource, /news_context: newsPayload/)
  })
})
