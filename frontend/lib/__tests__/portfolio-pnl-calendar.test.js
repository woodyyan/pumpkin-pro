import { describe, it } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const source = readFileSync(new URL('../portfolio-pnl-calendar.js', import.meta.url), 'utf8')
const transformedSource = source
  .replace(/import \{ requestJson \} from '\.\/api'\n/, '')
  .replace(/export /g, '')

const helpers = new Function(
  'requestJson',
  `${transformedSource}; return { fetchPortfolioPnlCalendar, formatCalendarPnlAmount, formatCalendarPnlRate, getCalendarPnlColor, resolveDefaultCalendarScope, resolveAvailableCalendarScopes };`
)((path, init, fallback) => Promise.resolve({ path, init, fallback }))

describe('portfolio pnl calendar helpers', () => {
  it('builds pnl calendar request with URLSearchParams', async () => {
    const result = await helpers.fetchPortfolioPnlCalendar({ scope: 'HKEX', year: 2026, month: 5 })
    assert.deepEqual(result, {
      path: '/api/portfolio/pnl-calendar?scope=HKEX&year=2026&month=5',
      init: undefined,
      fallback: '加载盈亏日历失败',
    })
  })

  it('formats A-share and HK pnl amounts with signs', () => {
    assert.equal(helpers.formatCalendarPnlAmount(1234.5, 'ASHARE'), '+¥1,234.50')
    assert.equal(helpers.formatCalendarPnlAmount(-123.45, 'HKEX'), '-HK$123.45')
  })

  it('formats missing pnl rate as placeholder', () => {
    assert.equal(helpers.formatCalendarPnlRate(null), '--')
    assert.equal(helpers.formatCalendarPnlRate(0.0123), '+1.23%')
    assert.equal(helpers.formatCalendarPnlRate(-0.0045), '-0.45%')
  })

  it('uses Chinese market pnl colors', () => {
    assert.equal(helpers.getCalendarPnlColor(1), 'text-rose-400')
    assert.equal(helpers.getCalendarPnlColor(-1), 'text-emerald-400')
    assert.equal(helpers.getCalendarPnlColor(0), 'text-white/42')
  })

  it('does not default mixed all-market calendars to ALL', () => {
    const summary = {
      scope: 'ALL',
      amounts_by_market: [
        { scope: 'ASHARE' },
        { scope: 'HKEX' },
      ],
    }
    assert.equal(helpers.resolveDefaultCalendarScope('ALL', summary), 'ASHARE')
    assert.deepEqual(helpers.resolveAvailableCalendarScopes(summary), ['ASHARE', 'HKEX'])
  })
})
