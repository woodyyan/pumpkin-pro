import { describe, it } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const pageSource = readFileSync(new URL('../../pages/portfolio.js', import.meta.url), 'utf8')

describe('portfolio market overview card structure', () => {
  it('keeps the key metric labels in MarketOverviewCard', () => {
    assert.match(pageSource, /总成本/)
    assert.match(pageSource, /今日盈亏/)
    assert.match(pageSource, /最大单仓占比/)
  })

  it('omits maximum single-position weight from the mixed all-market summary cards', () => {
    const summarySectionMatch = pageSource.match(/function SummarySection\(\{ summary \}\) \{[\s\S]*?^\}/m)
    if (!summarySectionMatch) {
      assert.fail('SummarySection not found')
    }
    const sectionBody = summarySectionMatch[0]
    const mixedBranch = sectionBody.split('if (isMixed) {')[1]?.split('const singleBlock = overviewBlocks[0] || null')[0]
    if (!mixedBranch) {
      assert.fail('mixed-market branch not found')
    }

    assert.doesNotMatch(mixedBranch, /<SummaryCard\s+label="最大单仓占比"/)
    assert.doesNotMatch(mixedBranch, /跨市场不做汇率折算/)
  })

  it('merges profit and loss counts into the overview UI', () => {
    assert.match(pageSource, /function ProfitLossCount/)
    assert.match(pageSource, /<ProfitLossCount profit=\{block\.profit_position_count\} loss=\{block\.loss_position_count\} \/>/)
    assert.match(pageSource, /<h3 className="text-sm font-semibold text-white\/80">分市场总览<\/h3>/)
    assert.match(pageSource, /<ProfitLossCount profit=\{summary\.profit_position_count\} loss=\{summary\.loss_position_count\} \/>/)
  })

  it('removes standalone auxiliary summary cards from SummarySection', () => {
    const summarySectionMatch = pageSource.match(/function SummarySection\(\{ summary \}\) \{[\s\S]*?^\}/m)
    if (!summarySectionMatch) {
      assert.fail('SummarySection not found')
    }
    const sectionBody = summarySectionMatch[0]

    assert.doesNotMatch(sectionBody, /label="持仓标的"/)
    assert.doesNotMatch(sectionBody, /label="盈利 \/ 亏损"/)
    assert.doesNotMatch(sectionBody, /label="账户总资金（设置）"/)
    assert.doesNotMatch(sectionBody, /label="资金利用率"/)
    assert.doesNotMatch(sectionBody, /label="最近交易"/)
  })

  it('merges unrealized, realized and total pnl into a single obvious selectable metric', () => {
    assert.match(pageSource, /MARKET_PNL_OPTIONS/)
    assert.match(pageSource, /selectedPnl/)
    assert.match(pageSource, /未实现盈亏/)
    assert.match(pageSource, /已实现盈亏/)
    assert.match(pageSource, /累计盈亏/)
    assert.match(pageSource, /border-primary\/35 bg-primary\/\[0\.10\]/)
    assert.match(pageSource, /focus-within:ring-2 focus-within:ring-primary\/20/)
    assert.match(pageSource, /aria-hidden="true">▾<\/span>/)
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

  it('removes the standalone single-market summary card grid', () => {
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
    assert.doesNotMatch(singleMarketBody, /grid-cols-2 md:grid-cols-4 gap-3/)
    assert.doesNotMatch(singleMarketBody, /<SummaryCard/)
    assert.match(singleMarketBody, /singleBlock \? <MarketOverviewCard block=\{singleBlock\} \/> : null/)
  })
})
