import { describe, it } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const pageSource = readFileSync(new URL('../../pages/live-trading/[symbol].js', import.meta.url), 'utf8')

describe('live trading signal summary section', () => {
  it('keeps folded state minimal and exposes only status plus config entry', () => {
    const foldedStart = pageSource.indexOf('交易信号')
    const expandedStart = pageSource.indexOf('signalConfigExpanded && (')
    assert.notEqual(foldedStart, -1)
    assert.notEqual(expandedStart, -1)
    const foldedSegment = pageSource.slice(foldedStart, expandedStart)

    assert.match(foldedSegment, /signalStatusSummary/)
    assert.match(foldedSegment, /shadow-\[0_0_0_1px_rgba\(110,231,183,0\.12\)\]/)
    assert.match(foldedSegment, /配置/)
    assert.doesNotMatch(foldedSegment, /role="switch"/)
    assert.doesNotMatch(foldedSegment, /signalConfigMeta.map/)
  })

  it('renders switch and meta cards only after expansion', () => {
    const expandedStart = pageSource.indexOf('signalConfigExpanded && (')
    const expandedSegment = pageSource.slice(expandedStart)

    assert.match(expandedSegment, /signalConfigMeta.map/)
    assert.match(expandedSegment, /mt-3 flex flex-wrap items-center gap-2\.5/)
    assert.match(expandedSegment, /role="switch"/)
  })
})
