import { describe, it } from 'node:test'
import assert from 'node:assert/strict'

import {
  AI_ANALYSIS_SHARE_MAX_IMAGE_HEIGHT,
  buildAIAnalysisShareFilename,
  buildAIAnalysisSharePayload,
  countAIAnalysisShareSlices,
  getAIAnalysisShareMarketLabel,
  getAIAnalysisSharePrimaryTimestamp,
  shouldUseServerShareFallback,
} from '../ai-analysis-share.js'

describe('ai-analysis-share helpers', () => {
  it('builds normalized payload with market label', () => {
    const payload = buildAIAnalysisSharePayload({
      symbol: '600519.SH',
      symbolName: '贵州茅台',
      exchange: 'SSE',
      result: { analysis: { signal: 'buy' }, meta: { generated_at: '2026-04-30T06:36:00.000Z' } },
    })

    assert.equal(payload.symbol, '600519.SH')
    assert.equal(payload.marketLabel, 'A股')
    assert.equal(getAIAnalysisSharePrimaryTimestamp(payload), '2026-04-30T06:36:00.000Z')
  })

  it('builds stable file names and appends slice suffix only when needed', () => {
    const payload = buildAIAnalysisSharePayload({
      symbol: '00700.HK',
      symbolName: '腾讯控股',
      exchange: 'HKEX',
      result: { analysis: { signal: 'hold', data_timestamp: '2026-04-30T06:35:00.000Z' }, meta: { generated_at: '2026-04-30T06:36:00.000Z' } },
    })

    assert.equal(buildAIAnalysisShareFilename(payload), 'AI分析-腾讯控股-00700.HK-2026-04-30-1436.png')
    assert.equal(buildAIAnalysisShareFilename(payload, 1, 3), 'AI分析-腾讯控股-00700.HK-2026-04-30-1436-2of3.png')
  })

  it('counts slices with threshold protection', () => {
    assert.equal(countAIAnalysisShareSlices(0), 1)
    assert.equal(countAIAnalysisShareSlices(AI_ANALYSIS_SHARE_MAX_IMAGE_HEIGHT), 1)
    assert.equal(countAIAnalysisShareSlices(AI_ANALYSIS_SHARE_MAX_IMAGE_HEIGHT + 1), 2)
    assert.equal(countAIAnalysisShareSlices(AI_ANALYSIS_SHARE_MAX_IMAGE_HEIGHT * 2 + 5), 3)
  })

  it('prefers server fallback for safari-like or extra tall content', () => {
    assert.equal(getAIAnalysisShareMarketLabel('SZSE'), 'A股')
    assert.equal(getAIAnalysisShareMarketLabel('HKEX'), '港股')

    assert.equal(shouldUseServerShareFallback({
      userAgent: 'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Safari/605.1.15',
      elementHeight: 1200,
    }), true)

    assert.equal(shouldUseServerShareFallback({
      userAgent: 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36',
      elementHeight: AI_ANALYSIS_SHARE_MAX_IMAGE_HEIGHT + 10,
    }), true)

    assert.equal(shouldUseServerShareFallback({
      userAgent: 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36',
      elementHeight: 1200,
    }), false)
  })
})
