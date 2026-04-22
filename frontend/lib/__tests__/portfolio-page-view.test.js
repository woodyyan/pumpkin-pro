import { describe, it } from 'node:test'
import assert from 'node:assert/strict'

import { getPortfolioPageViewState } from '../portfolio-page-view.js'

describe('getPortfolioPageViewState', () => {
  it('shows initial skeleton only before any data is available', () => {
    const state = getPortfolioPageViewState({ loading: true, data: null })

    assert.equal(state.initialLoading, true)
    assert.equal(state.hasDashboardData, false)
    assert.equal(state.refreshing, false)
  })

  it('keeps previously loaded content visible during refresh', () => {
    const state = getPortfolioPageViewState({ loading: true, data: { summary: { position_count: 3 } } })

    assert.equal(state.initialLoading, false)
    assert.equal(state.hasDashboardData, true)
    assert.equal(state.refreshing, true)
  })

  it('marks settled content as non-refreshing after load completes', () => {
    const state = getPortfolioPageViewState({ loading: false, data: { summary: { position_count: 3 } } })

    assert.equal(state.initialLoading, false)
    assert.equal(state.hasDashboardData, true)
    assert.equal(state.refreshing, false)
  })
})
