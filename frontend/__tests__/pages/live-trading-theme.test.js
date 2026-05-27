import { describe, it } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const pageSource = readFileSync(new URL('../../pages/live-trading/[symbol].js', import.meta.url), 'utf8')

describe('live trading chart theme integration', () => {
  it('uses shared chart theme helpers instead of leaked local constants', () => {
    assert.match(pageSource, /const CHART_CONFIG =/)
    assert.match(pageSource, /function pickChartTheme\(resolvedTheme\)/)
    assert.doesNotMatch(pageSource, /CHART_THEME/)
  })

  it('wires all chart backgrounds to pickChartTheme', () => {
    const backgroundMatches = pageSource.match(/color: pickChartTheme\(resolvedTheme\)\.background/g) || []
    assert.ok(backgroundMatches.length >= 6, `expected themed chart backgrounds, got ${backgroundMatches.length}`)
  })

  it('reads resolvedTheme inside chart components', () => {
    const themeHookMatches = pageSource.match(/const \{ resolvedTheme \} = useTheme\(\)/g) || []
    assert.ok(themeHookMatches.length >= 7, `expected chart components to read theme, got ${themeHookMatches.length}`)
  })
})
