import { describe, it } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const pageSource = readFileSync(new URL('../../pages/ai/analysis.js', import.meta.url), 'utf8')
const helperSource = readFileSync(new URL('../../lib/ai-analysis-helpers.js', import.meta.url), 'utf8')
const historySource = readFileSync(new URL('../../components/AIAnalysisHistorySection.js', import.meta.url), 'utf8')

describe('/ai/analysis page structure', () => {
  it('uses shared AI analysis workspace and global history section', () => {
    assert.match(pageSource, /AIAnalysisEntryForm/)
    assert.match(pageSource, /AIAnalysisCapabilityCards/)
    assert.match(pageSource, /GlobalAIAnalysisHistorySection/)
    assert.match(pageSource, /AIAnalysisPanel/)
  })

  it('supports ?symbol prefill but does not auto trigger analysis', () => {
    assert.match(pageSource, /router\.query\.symbol/)
    assert.match(pageSource, /resolveAnalysisTarget\(urlSymbol/)
    assert.doesNotMatch(pageSource, /handleSubmit\(\)\s*\n\s*}\s*, \[ready, router\.isReady, router\.query\.symbol\]/)
  })

  it('requires login before analysis and loads paginated global history', () => {
    assert.match(pageSource, /登录后可使用 AI 分析功能/)
    assert.match(pageSource, /fetchGlobalAIAnalysisHistory/)
    assert.match(pageSource, /AI_ANALYSIS_GLOBAL_HISTORY_PAGE_SIZE/)
  })
})

describe('shared AI analysis helpers', () => {
  it('reuses /api/search for target lookup', () => {
    assert.match(helperSource, /requestJson\(`\/api\/search\?q=/)
    assert.doesNotMatch(helperSource, /\/api\/ai-analysis\/search/)
  })

  it('exposes global history endpoint helper', () => {
    assert.match(helperSource, /fetchGlobalAIAnalysisHistory/)
    assert.match(helperSource, /\/api\/ai-analysis\/history\?page=/)
  })
})

describe('AI analysis history presentation', () => {
  it('renders quality validation summary in global history cards', () => {
    assert.match(historySource, /最近 AI 分析历史/)
    assert.match(historySource, /5 日验证等质量结果/)
    assert.match(historySource, /buildQualityValidationHeadline/)
    assert.match(historySource, /HistoryQualitySummary/)
  })
})
