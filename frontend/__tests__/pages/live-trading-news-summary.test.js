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

  it('uses the watch-style CTA copy for opening the full news panel', () => {
    const cardSource = readFileSync(new URL('../../components/SymbolNewsSummaryCard.js', import.meta.url), 'utf8')
    assert.match(cardSource, /查看全部 →/)
    assert.match(cardSource, /border-primary\/40 bg-primary\/10/)
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

  it('surfaces news loading state inside the AI wait panel', () => {
    assert.match(pageSource, /aiNewsContextState/)
    assert.match(pageSource, /新闻上下文/)
    assert.match(pageSource, /最近的媒体新闻、公司公告和财报/)
  })
})
