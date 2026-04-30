import { describe, it } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const symbolPageSource = readFileSync(new URL('../../pages/live-trading/[symbol].js', import.meta.url), 'utf8')
const shareCardSource = readFileSync(new URL('../../components/AIAnalysisShareCard.js', import.meta.url), 'utf8')
const shareLibSource = readFileSync(new URL('../../lib/ai-analysis-share.js', import.meta.url), 'utf8')
const sharePreviewSource = readFileSync(new URL('../../pages/share/ai-analysis-preview.js', import.meta.url), 'utf8')
const shareApiSource = readFileSync(new URL('../../pages/api/share/ai-analysis-image.js', import.meta.url), 'utf8')
const appSource = readFileSync(new URL('../../pages/_app.js', import.meta.url), 'utf8')

describe('AI analysis share image integration', () => {
  it('adds the generate image action on the current AI analysis result panel', () => {
    assert.match(symbolPageSource, /生成图片/)
    assert.match(symbolPageSource, /exportAIAnalysisShareImages/)
    assert.match(symbolPageSource, /buildAIAnalysisSharePayload/)
    assert.match(symbolPageSource, /<AIAnalysisShareCard payload=\{sharePayload\}/)
  })

  it('keeps historical analysis export out of scope', () => {
    const historyPanelStart = symbolPageSource.indexOf('function AnalysisHistoryPanel')
    assert.notEqual(historyPanelStart, -1)
    const historySegment = symbolPageSource.slice(historyPanelStart, historyPanelStart + 5000)
    assert.doesNotMatch(historySegment, /生成图片/)
  })

  it('renders a branded share card with stock info and privacy-safe defaults', () => {
    assert.match(shareCardSource, /卧龙AI量化交易台/)
    assert.match(shareCardSource, /wolongtrader\.top/)
    assert.match(shareCardSource, /分析时间/)
    assert.match(shareCardSource, /hidePositionHint/)
  })

  it('provides client export plus server fallback and long-image protection', () => {
    assert.match(shareLibSource, /html-to-image/)
    assert.match(shareLibSource, /shouldUseServerShareFallback/)
    assert.match(shareLibSource, /countAIAnalysisShareSlices/)
    assert.match(shareLibSource, /\/api\/share\/ai-analysis-image/)
    assert.match(shareApiSource, /puppeteer-core/)
    assert.match(shareApiSource, /AI_ANALYSIS_SHARE_MAX_IMAGE_HEIGHT/)
    assert.match(shareApiSource, /buildAIAnalysisShareFilename/)
  })

  it('adds an internal preview page and bypasses the normal app layout for screenshots', () => {
    assert.match(sharePreviewSource, /ai-analysis-share-payload/)
    assert.match(sharePreviewSource, /data-share-ready/)
    assert.match(appSource, /\/share\/ai-analysis-preview/)
  })
})
