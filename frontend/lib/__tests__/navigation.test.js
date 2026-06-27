import { describe, it } from 'node:test'
import assert from 'node:assert/strict'

import { NAV_GROUPS, buildNavigationState, formatNavBadgeCount } from '../navigation.js'

describe('navigation config', () => {
  it('uses the approved semantic routes for new placeholder pages', () => {
    const hrefs = NAV_GROUPS.flatMap((group) => group.items.map((item) => item.href))

    assert.ok(hrefs.includes('/ai/analysis'))
    assert.ok(hrefs.includes('/ai/reports'))
    assert.ok(hrefs.includes('/ai/picker'))
    assert.ok(hrefs.includes('/ai/backtest'))
    assert.ok(hrefs.includes('/quadrant'))
    assert.ok(hrefs.includes('/watchlist'))
    assert.ok(hrefs.includes('/portfolio-tracking'))
    assert.ok(hrefs.includes('/live-trading'))
  })

  it('places AI reports between AI analysis and AI picker', () => {
    const aiItems = NAV_GROUPS.find((group) => group.key === 'wolong-ai').items

    assert.deepEqual(aiItems.map((item) => item.href).slice(0, 3), ['/ai/analysis', '/ai/reports', '/ai/picker'])
  })

  it('does not duplicate hrefs across navigation items', () => {
    const hrefs = NAV_GROUPS.flatMap((group) => group.items.map((item) => item.href))
    assert.equal(new Set(hrefs).size, hrefs.length)
  })

  it('returns null badges for zero and formats positive counts', () => {
    assert.equal(formatNavBadgeCount(0), null)
    assert.equal(formatNavBadgeCount(3), '3')
    assert.equal(formatNavBadgeCount(108), '99+')
  })

  it('keeps non-navigation pages without an active top-level group', () => {
    const state = buildNavigationState('/settings', 0)
    assert.equal(state.activeGroupKey, null)
    assert.equal(state.groups.some((group) => group.isActive), false)
  })
})
