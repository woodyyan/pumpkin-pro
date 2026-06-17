import { describe, it } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const pageSource = readFileSync(new URL('../../pages/ai/picker.js', import.meta.url), 'utf8')

describe('/ai/picker page load behavior', () => {
  it('uses attempted-market guard to avoid repeated auto reload after failures', () => {
    assert.match(pageSource, /attemptedByMarket/)
    assert.match(pageSource, /markMarketLoadAttempted/)
    assert.match(pageSource, /shouldAutoLoadAIPickerMarket/)
    assert.doesNotMatch(pageSource, /!metaByMarket\[market\]/)
    assert.doesNotMatch(pageSource, /!resultByMarket\[market\] && !loadingByMarket\[market\]/)
  })

  it('does not render a manual retry button in the error state', () => {
    assert.match(pageSource, /error && !analysis/)
    assert.doesNotMatch(pageSource, />重试加载</)
    assert.doesNotMatch(pageSource, /onClick=\{\(\) => loadPage\(market\)\}/)
  })
})
