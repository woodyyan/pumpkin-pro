import { describe, it } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const pageSource = readFileSync(new URL('../../pages/live-trading/[symbol].js', import.meta.url), 'utf8')

describe('live trading chart theme integration', () => {
  it('keeps detail page wired to the shared theme context', () => {
    assert.match(pageSource, /import \{ useTheme \} from '\.\.\/\.\.\/lib\/theme-context'/)
    assert.match(pageSource, /const \{ resolvedTheme \} = useTheme\(\)/)
  })

  it('does not rely on the removed old chart theme implementation contract', () => {
    assert.doesNotMatch(pageSource, /const CHART_CONFIG =/)
    assert.doesNotMatch(pageSource, /function pickChartTheme\(resolvedTheme\)/)
  })

  it('still threads resolvedTheme through the live trading detail page after refactor', () => {
    assert.match(pageSource, /resolvedTheme/)
  })
})
