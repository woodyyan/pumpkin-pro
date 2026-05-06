import { describe, it } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const pageSource = readFileSync(new URL('../../pages/live-trading/[symbol].js', import.meta.url), 'utf8')

describe('live trading company about integration', () => {
  it('adds CompanyAboutPanel and lazy about loader without joining symbol panel polling', () => {
    assert.match(pageSource, /CompanyAboutPanel/)
    assert.match(pageSource, /fetchCompanyAbout/)
    assert.match(pageSource, /const loadCompanyAbout = async/)

    const symbolPanelsStart = pageSource.indexOf('const loadSymbolPanels')
    const symbolPanelsEnd = pageSource.indexOf('const loadSupportLevels')
    const symbolPanelsSegment = pageSource.slice(symbolPanelsStart, symbolPanelsEnd)
    assert.doesNotMatch(symbolPanelsSegment, /fetchCompanyAbout|loadCompanyAbout/)
  })

  it('keeps about entry hidden behind a feature flag while preserving the implementation', () => {
    assert.match(pageSource, /const SHOW_COMPANY_ABOUT_ENTRY = false/)
    assert.match(pageSource, /SHOW_COMPANY_ABOUT_ENTRY \? \(/)
  })

  it('keeps about button implementation in identity action group near watch button for future re-enable', () => {
    const titleStart = pageSource.indexOf('{symbolName ? `${symbolName}（${symbol}）` : symbol}')
    const metaStart = pageSource.indexOf('<div className="mt-1 flex items-center gap-3 text-xs text-white/55">', titleStart)
    const identitySegment = pageSource.slice(titleStart, metaStart)

    assert.match(identitySegment, /\+ 关注/)
    assert.match(identitySegment, /关于/)
    assert.match(identitySegment, /aria-expanded=\{aboutOpen \? 'true' : 'false'\}/)
  })

  it('keeps AI analysis as a separate primary button', () => {
    const headerStart = pageSource.indexOf('{symbolName ? `${symbolName}（${symbol}）` : symbol}')
    const aboutIndex = pageSource.indexOf('toggleCompanyAbout', headerStart)
    const aiIndex = pageSource.indexOf('handleAIAnalysis', headerStart)
    assert.ok(aboutIndex > 0)
    assert.ok(aiIndex > aboutIndex)

    const aiSegment = pageSource.slice(aiIndex, aiIndex + 1200)
    assert.match(aiSegment, /AI 综合分析该股票/)
    assert.match(aiSegment, /bg-gradient-to-r from-indigo-500 to-violet-500/)
  })

  it('resets about state when symbol changes', () => {
    const resetStart = pageSource.indexOf("setPortfolioAction('')")
    const resetEnd = pageSource.indexOf('newsSummaryRefreshRef.current', resetStart)
    const resetSegment = pageSource.slice(resetStart, resetEnd)

    assert.match(resetSegment, /setAboutOpen\(false\)/)
    assert.match(resetSegment, /setAboutPayload\(null\)/)
    assert.match(resetSegment, /setAboutLoadedSymbol\(''\)/)
  })
})
