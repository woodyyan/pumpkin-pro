import { describe, it } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const pageSource = readFileSync(new URL('../../pages/portfolio.js', import.meta.url), 'utf8')

describe('portfolio market overview card structure', () => {
  it('keeps the 4 metric labels in MarketOverviewCard', () => {
    assert.match(pageSource, /总成本/)
    assert.match(pageSource, /今日盈亏/)
    assert.match(pageSource, /最大单仓占比/)
  })

  it('merges unrealized, realized and total pnl into a single selectable metric', () => {
    assert.match(pageSource, /MARKET_PNL_OPTIONS/)
    assert.match(pageSource, /selectedPnl/)
    assert.match(pageSource, /未实现盈亏/)
    assert.match(pageSource, /已实现盈亏/)
    assert.match(pageSource, /累计盈亏/)
  })

  it('defaults the selectable pnl metric to total_pnl', () => {
    assert.match(pageSource, /useState\('total_pnl'\)/)
  })

  it('uses a 4-column grid on desktop and 2-column on mobile for the market card metrics', () => {
    assert.match(pageSource, /grid-cols-2 md:grid-cols-4 gap-2\.5/)
  })

  it('does not render all 6 original summary rows together in MarketOverviewCard', () => {
    const marketCardMatch = pageSource.match(/function MarketOverviewCard\(\{ block \}\) \{[\s\S]*?^\}/m)
    if (!marketCardMatch) {
      assert.fail('MarketOverviewCard not found')
    }
    const cardBody = marketCardMatch[0]
    assert.doesNotMatch(cardBody, /label="未实现盈亏"[^>]*>\s*<SummaryRow/)
    assert.doesNotMatch(cardBody, /label="已实现盈亏"[^>]*>\s*<SummaryRow/)
    assert.doesNotMatch(cardBody, /label="累计盈亏"[^>]*>\s*<SummaryRow/)
  })

  it('reuses MarketOverviewCard for single-market summaries', () => {
    const summarySectionMatch = pageSource.match(/function SummarySection\(\{ summary \}\) \{[\s\S]*?^\}/m)
    if (!summarySectionMatch) {
      assert.fail('SummarySection not found')
    }
    const sectionBody = summarySectionMatch[0]

    assert.match(sectionBody, /const overviewBlocks = buildPortfolioOverviewBlocks\(summary\)/)
    assert.match(sectionBody, /singleBlock \? <MarketOverviewCard block=\{singleBlock\} \/> : null/)
  })

  it('removes the standalone 6-card single-market metric grid', () => {
    const summarySectionMatch = pageSource.match(/function SummarySection\(\{ summary \}\) \{[\s\S]*?^\}/m)
    if (!summarySectionMatch) {
      assert.fail('SummarySection not found')
    }
    const sectionBody = summarySectionMatch[0]
    const singleMarketBody = sectionBody.split('const singleBlock = overviewBlocks[0] || null')[1]
    if (!singleMarketBody) {
      assert.fail('single-market branch not found')
    }

    assert.doesNotMatch(singleMarketBody, /grid-cols-2 md:grid-cols-3 xl:grid-cols-6/)
    assert.doesNotMatch(singleMarketBody, /SummaryCard label="未实现盈亏"/)
    assert.doesNotMatch(singleMarketBody, /SummaryCard label="已实现盈亏"/)
    assert.doesNotMatch(singleMarketBody, /SummaryCard label="累计盈亏"/)
    assert.doesNotMatch(singleMarketBody, /SummaryCard\s+label="最大单仓占比"\s+tooltip=\{FIELD_TIPS\.max_position_weight\}/)
  })
})
