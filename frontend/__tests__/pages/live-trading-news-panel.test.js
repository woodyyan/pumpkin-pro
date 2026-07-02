import { describe, it } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const pageSource = readFileSync(new URL('../../pages/live-trading/[symbol].js', import.meta.url), 'utf8')
const panelSource = readFileSync(new URL('../../components/SymbolNewsPanel.js', import.meta.url), 'utf8')

describe('live trading news tab integration', () => {
  it('renders news and announcements inline under the news tab', () => {
    assert.match(pageSource, /function InlineSymbolNewsList/)
    assert.match(pageSource, /<InlineSymbolNewsList/)
    assert.match(pageSource, /新闻与公告列表/)
    assert.match(pageSource, /onTypeChange=\{setNewsFilter\}/)
    assert.match(pageSource, /const NEWS_FILTERS = \[/)
    assert.match(pageSource, /全部/)
    assert.match(pageSource, /公告/)
    assert.match(pageSource, /财报/)
  })

  it('removes the side drawer from the detail page while keeping the reusable component available', () => {
    assert.doesNotMatch(pageSource, /<SymbolNewsPanel/)
    assert.doesNotMatch(pageSource, /newsPanelOpen/)
    assert.doesNotMatch(pageSource, /openNewsPanel/)
    assert.match(panelSource, /fixed inset-0 z-\[70\]/)
    assert.match(panelSource, /md:w-\[460px\]/)
  })
})
