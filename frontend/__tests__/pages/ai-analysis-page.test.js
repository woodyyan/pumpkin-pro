import { describe, it } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const pageSource = readFileSync(new URL('../../pages/ai/analysis.js', import.meta.url), 'utf8')
const helperSource = readFileSync(new URL('../../lib/ai-analysis-helpers.js', import.meta.url), 'utf8')
const historySource = readFileSync(new URL('../../components/AIAnalysisHistorySection.js', import.meta.url), 'utf8')
const workspaceSource = readFileSync(new URL('../../components/AIAnalysisWorkspace.js', import.meta.url), 'utf8')

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

  it('builds AI analysis context from returned dependencies instead of stale page state', () => {
    assert.match(pageSource, /const dependencies = await loadAnalysisDependencies\(target\)/)
    assert.match(pageSource, /snapshotPayload: dependencies\.snapshotPayload/)
    assert.match(pageSource, /movingAveragePayload: dependencies\.movingAveragePayload/)
    assert.match(pageSource, /fundamentalsItems: dependencies\.fundamentalsItems/)
    assert.match(pageSource, /portfolioData: dependencies\.portfolioData/)
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

  it('exposes shared dependency loading helpers for analysis pages', () => {
    assert.match(helperSource, /export async function loadAIAnalysisDependencies/)
    assert.match(helperSource, /fetchSymbolSnapshot\(symbol\)/)
    assert.match(helperSource, /fetchSymbolDailyBars\(symbol, 240\)/)
    assert.match(helperSource, /deriveMovingAveragePayloadFromBars\(bars\)/)
  })

  it('normalizes snapshot and daily bars into the structure expected by the shared context builder', () => {
    assert.match(helperSource, /const \[snapshot, dailyBars, fundamentalsData, portfolioRes\] = await Promise\.all\(\[/)
    assert.match(helperSource, /const bars = Array\.isArray\(dailyBars\) \? dailyBars : \[\]/)
    assert.match(helperSource, /snapshotPayload: snapshot \? \{ snapshot \} : null/)
    assert.doesNotMatch(helperSource, /dailyBarsData\?\.bars/)
  })
})

describe('AI analysis history presentation', () => {
  it('renders quality validation summary in global history cards', () => {
    assert.match(historySource, /最近 AI 分析历史/)
    assert.match(historySource, /5 日验证等质量结果/)
    assert.match(historySource, /buildQualityValidationHeadline/)
    assert.match(historySource, /HistoryQualitySummary/)
  })

  it('supports inline detail expansion and symbol link navigation in global history cards', () => {
    assert.match(historySource, /Link from 'next\/link'/)
    assert.match(historySource, /onClick=\{\(event\) => event\.stopPropagation\(\)\}/)
    assert.match(historySource, /useHistoryDetailController/)
    assert.match(historySource, /\/api\/live\/symbols\/\$\{encodeURIComponent\(item\.symbol\)\}\/analysis-history\?id=\$\{encodeURIComponent\(item\.id\)\}/)
    assert.match(historySource, /detailCache/)
    assert.match(historySource, /detailErrorById/)
  })

  it('matches the live-trading AI button visual style', () => {
    assert.match(workspaceSource, /title="AI 综合分析该股票"/)
    assert.match(workspaceSource, /bg-gradient-to-r from-indigo-500 to-violet-500/)
    assert.match(workspaceSource, /shadow-\[0_0_16px_rgba\(99,102,241,0\.35\)\]/)
    assert.match(workspaceSource, /hover:scale-\[1\.03\]/)
    assert.match(workspaceSource, /animate-ai-glow/)
    assert.match(workspaceSource, /✨ AI 分析/)
  })

  it('uses stronger light-mode colors for login prompt and history badges', () => {
    assert.match(pageSource, /AIAnalysisEntryForm/)
    assert.match(workspaceSource, /text-amber-800 dark:text-amber-100\/90/)
    assert.match(workspaceSource, /text-amber-900 underline underline-offset-2 dark:text-inherit/)
    assert.match(historySource, /text-amber-700 dark:text-amber-300/)
    assert.match(historySource, /text-amber-800 dark:text-amber-200 bg-amber-500\/10 border-amber-400\/25/)
  })
})
