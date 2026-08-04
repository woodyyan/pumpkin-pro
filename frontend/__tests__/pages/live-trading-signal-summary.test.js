import { describe, it } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const pageSource = readFileSync(new URL('../../pages/live-trading/[symbol].js', import.meta.url), 'utf8')

describe('live trading signal summary section', () => {
  it('keeps a read-only signal summary with entry to signal center inside portfolio tab', () => {
    const signalStart = pageSource.indexOf('信号摘要（完整配置已迁入信号中心')
    assert.notEqual(signalStart, -1)
    const signalSegment = pageSource.slice(signalStart, pageSource.indexOf('{isPortfolioTab && !privateAccessReady', signalStart))

    assert.match(signalSegment, /交易信号/)
    assert.match(signalSegment, /signalSummaryText/)
    assert.match(signalSegment, /\/signals\?symbol=/)
    assert.match(signalSegment, /管理信号|开启信号/)
  })

  it('no longer embeds the legacy inline signal config editor', () => {
    assert.ok(!pageSource.includes('Inline signal config'))
    assert.ok(!pageSource.includes('signalConfigMeta'))
    assert.ok(!pageSource.includes('handleToggleSignal'))
    assert.ok(!pageSource.includes('handleSaveSignalConfig'))
    assert.ok(!pageSource.includes('lib/signal-config-ui'))
    assert.ok(!pageSource.includes("/api/signal-configs"))
  })

  it('loads per-symbol subscription summary from the signal center API', () => {
    assert.ok(pageSource.includes("requestJson('/api/signal/subscriptions')"))
    assert.ok(pageSource.includes('loadSignalSummary'))
    assert.ok(pageSource.includes('lib/signal-center-ui'))
  })
})
