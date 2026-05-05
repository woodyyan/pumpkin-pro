import { describe, it } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const pageSource = readFileSync(new URL('../../pages/portfolio.js', import.meta.url), 'utf8')
const allocationPieStart = pageSource.indexOf('function AllocationPie')
const allocationPieEnd = pageSource.indexOf('function PortfolioChartsSection')
const allocationPieSource = allocationPieStart >= 0 && allocationPieEnd > allocationPieStart
  ? pageSource.slice(allocationPieStart, allocationPieEnd)
  : ''

describe('portfolio allocation pie chart', () => {
  it('renders allocation as a donut pie chart instead of a horizontal progress list', () => {
    assert.ok(allocationPieSource, 'AllocationPie component not found')
    assert.match(pageSource, /const ALLOCATION_PIE_COLORS = \[/)
    assert.match(allocationPieSource, /<svg viewBox="0 0 220 220"[^>]*aria-label="持仓分布环形饼图"/)
    assert.match(allocationPieSource, /fill="none"/)
    assert.match(allocationPieSource, /strokeDasharray=\{`\$\{\(ratio \* circumference\)\.toFixed\(3\)\} \$\{circumference\.toFixed\(3\)\}`\}/)
    assert.match(allocationPieSource, /stroke="transparent"/)
    assert.doesNotMatch(allocationPieSource, /buildAllocationPiePath/)
    assert.doesNotMatch(allocationPieSource, /style=\{\{ width: pctStr \}\}/)
  })

  it('keeps the existing allocation data and stock navigation in the legend', () => {
    assert.match(allocationPieSource, /const totalMarketValue = allocationItems\.reduce/)
    assert.match(allocationPieSource, /const pctStr = `\$\{\(ratio \* 100\)\.toFixed\(1\)\}%`/)
    assert.match(allocationPieSource, /<Link href=\{`\/live-trading\/\$\{item\.symbol\}`\}/)
    assert.match(allocationPieSource, /formatCompactNumber\(item\.market_value_amount\)/)
    assert.match(allocationPieSource, /exchangeTag\(item\.exchange\)/)
  })

  it('uses AllocationPie in the portfolio charts section', () => {
    assert.match(pageSource, /<AllocationPie allocationItems=\{allocationItems\} \/>/)
    assert.doesNotMatch(pageSource, /<AllocationBar allocationItems=\{allocationItems\} \/>/)
  })
})
