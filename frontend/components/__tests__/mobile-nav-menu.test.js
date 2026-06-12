import { describe, it } from 'node:test'
import assert from 'node:assert/strict'

import { buildNavigationState } from '../../lib/navigation.js'

describe('mobile nav grouped menu', () => {
  it('uses the new first-level group order and keeps tracking children together', () => {
    const state = buildNavigationState('/watchlist', 0)

    assert.deepEqual(state.groups.map((group) => group.label), ['卧龙AI', '看板', '跟踪', '选股', '更多'])
    assert.deepEqual(
      state.groups.find((group) => group.key === 'tracking').items.map((item) => item.label),
      ['自选股', '持仓管理']
    )
    assert.equal(state.activeGroupKey, 'tracking')
  })

  it('treats live-trading detail pages as the market entry under 看板', () => {
    const state = buildNavigationState('/live-trading/00700', 0)
    const dashboardGroup = state.groups.find((group) => group.key === 'dashboard')
    const marketItem = dashboardGroup.items.find((item) => item.key === 'market-overview')

    assert.equal(dashboardGroup.isActive, true)
    assert.equal(marketItem.isActive, true)
    assert.equal(state.activeGroupKey, 'dashboard')
  })
})
