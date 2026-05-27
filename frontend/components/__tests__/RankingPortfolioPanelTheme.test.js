import { describe, it } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const source = readFileSync(new URL('../RankingPortfolioPanel.js', import.meta.url), 'utf8')

describe('RankingPortfolioPanel theme integration', () => {
  it('uses theme-aware chart axis text colors for light and dark mode', () => {
    assert.match(source, /import \{ useTheme \} from '\.\.\/lib\/theme-context'/)
    assert.match(source, /const \{ resolvedTheme \} = useTheme\(\)/)
    assert.match(source, /textLight: 'rgba\(30,41,59,0\.78\)'/)
    assert.match(source, /textDark: 'rgba\(255,255,255,0\.72\)'/)
    assert.match(source, /textColor: resolvedTheme === 'light' \? CHART_COLORS\.textLight : CHART_COLORS\.textDark/)
  })
})
