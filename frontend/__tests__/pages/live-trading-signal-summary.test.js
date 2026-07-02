import { describe, it } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const pageSource = readFileSync(new URL('../../pages/live-trading/[symbol].js', import.meta.url), 'utf8')

describe('live trading signal summary section', () => {
  it('keeps trading signal configuration visible by default inside portfolio tab', () => {
    const signalStart = pageSource.indexOf('Inline signal config')
    assert.notEqual(signalStart, -1)
    const signalSegment = pageSource.slice(signalStart, pageSource.indexOf('{isPortfolioTab && !privateAccessReady', signalStart))

    assert.match(signalSegment, /交易信号/)
    assert.match(signalSegment, /signalStatusSummary/)
    assert.match(signalSegment, /signalConfigMeta.map/)
    assert.match(signalSegment, /role="switch"/)
    assert.match(signalSegment, /保存配置/)
  })

  it('removes the expand or collapse state and button for signal config', () => {
    assert.doesNotMatch(pageSource, /signalConfigExpanded/)
    assert.doesNotMatch(pageSource, /展开配置|收起配置/)
  })
})
