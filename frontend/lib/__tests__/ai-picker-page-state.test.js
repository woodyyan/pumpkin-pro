import { describe, it } from 'node:test'
import assert from 'node:assert/strict'

import { markMarketLoadAttempted, shouldAutoLoadAIPickerMarket } from '../ai-picker-page-state.js'

describe('ai picker page state', () => {
  it('marks the current market as attempted only once', () => {
    const initial = markMarketLoadAttempted(undefined, 'ASHARE')
    assert.deepEqual(initial, { ASHARE: true })

    const repeated = markMarketLoadAttempted(initial, 'ASHARE')
    assert.equal(repeated, initial)

    const next = markMarketLoadAttempted(initial, 'HKEX')
    assert.deepEqual(next, { ASHARE: true, HKEX: true })
  })

  it('allows auto load before the first request starts', () => {
    assert.equal(
      shouldAutoLoadAIPickerMarket({
        market: 'ASHARE',
        attemptedByMarket: {},
        loadingByMarket: {},
      }),
      true
    )
  })

  it('blocks auto load when the market is already loading', () => {
    assert.equal(
      shouldAutoLoadAIPickerMarket({
        market: 'ASHARE',
        attemptedByMarket: {},
        loadingByMarket: { ASHARE: true },
      }),
      false
    )
  })

  it('blocks auto load after a failed or completed first attempt', () => {
    assert.equal(
      shouldAutoLoadAIPickerMarket({
        market: 'ASHARE',
        attemptedByMarket: { ASHARE: true },
        loadingByMarket: { ASHARE: false },
      }),
      false
    )
  })

  it('returns false for empty market input', () => {
    assert.equal(
      shouldAutoLoadAIPickerMarket({
        market: '',
        attemptedByMarket: {},
        loadingByMarket: {},
      }),
      false
    )
  })
})
