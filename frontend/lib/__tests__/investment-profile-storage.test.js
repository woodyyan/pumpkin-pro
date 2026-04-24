import { beforeEach, describe, it } from 'node:test'
import assert from 'node:assert/strict'

const INVESTMENT_PROFILE_STORAGE_KEY = 'pumpkin_pro_investment_profile'

let store = {}
let listeners = new Map()

function mockWindow() {
  return {
    localStorage: {
      getItem(key) {
        return Object.prototype.hasOwnProperty.call(store, key) ? store[key] : null
      },
      setItem(key, value) {
        store[key] = String(value)
      },
      removeItem(key) {
        delete store[key]
      },
    },
    addEventListener(type, handler) {
      if (!listeners.has(type)) listeners.set(type, new Set())
      listeners.get(type).add(handler)
    },
    removeEventListener(type, handler) {
      listeners.get(type)?.delete(handler)
    },
    dispatchEvent(event) {
      for (const handler of listeners.get(event.type) || []) {
        handler(event)
      }
      return true
    },
  }
}

class MockCustomEvent {
  constructor(type, init = {}) {
    this.type = type
    this.detail = init.detail
  }
}

function readInvestmentProfileCache() {
  const text = window.localStorage.getItem(INVESTMENT_PROFILE_STORAGE_KEY)
  if (!text) return null
  try {
    const parsed = JSON.parse(text)
    return parsed && typeof parsed === 'object' ? parsed : null
  } catch {
    return null
  }
}

function writeInvestmentProfileCache(profile) {
  if (!profile || typeof profile !== 'object') {
    window.localStorage.removeItem(INVESTMENT_PROFILE_STORAGE_KEY)
    return
  }
  window.localStorage.setItem(INVESTMENT_PROFILE_STORAGE_KEY, JSON.stringify(profile))
}

function clearInvestmentProfileCache() {
  window.localStorage.removeItem(INVESTMENT_PROFILE_STORAGE_KEY)
}

function dispatchInvestmentProfileUpdated(profile) {
  window.dispatchEvent(new CustomEvent('pumpkin:investment-profile-updated', { detail: profile || null }))
}

function subscribeInvestmentProfileUpdates(listener) {
  const handleCustomEvent = (event) => {
    listener(event?.detail || null)
  }
  const handleStorageEvent = (event) => {
    if (event.key !== INVESTMENT_PROFILE_STORAGE_KEY) return
    if (!event.newValue) {
      listener(null)
      return
    }
    try {
      const parsed = JSON.parse(event.newValue)
      listener(parsed && typeof parsed === 'object' ? parsed : null)
    } catch {
      listener(null)
    }
  }
  window.addEventListener('pumpkin:investment-profile-updated', handleCustomEvent)
  window.addEventListener('storage', handleStorageEvent)
  return () => {
    window.removeEventListener('pumpkin:investment-profile-updated', handleCustomEvent)
    window.removeEventListener('storage', handleStorageEvent)
  }
}

describe('investment-profile-storage helpers', () => {
  beforeEach(() => {
    store = {}
    listeners = new Map()
    global.window = mockWindow()
    global.CustomEvent = MockCustomEvent
  })

  it('reads and writes cached profile', () => {
    writeInvestmentProfileCache({ default_fee_rate_ashare_buy: 0.0002 })
    assert.deepEqual(readInvestmentProfileCache(), { default_fee_rate_ashare_buy: 0.0002 })
  })

  it('clears cached profile', () => {
    writeInvestmentProfileCache({ default_fee_rate_ashare_buy: 0.0002 })
    clearInvestmentProfileCache()
    assert.equal(readInvestmentProfileCache(), null)
  })

  it('notifies same-tab listeners via custom event', () => {
    let payload = null
    const unsubscribe = subscribeInvestmentProfileUpdates((profile) => {
      payload = profile
    })
    dispatchInvestmentProfileUpdated({ default_fee_rate_hk_buy: 0.0011 })
    unsubscribe()
    assert.deepEqual(payload, { default_fee_rate_hk_buy: 0.0011 })
  })

  it('notifies cross-tab listeners via storage event', () => {
    let payload = 'unset'
    const unsubscribe = subscribeInvestmentProfileUpdates((profile) => {
      payload = profile
    })
    window.dispatchEvent({
      type: 'storage',
      key: INVESTMENT_PROFILE_STORAGE_KEY,
      newValue: JSON.stringify({ default_fee_rate_ashare_sell: 0.0007 }),
    })
    unsubscribe()
    assert.deepEqual(payload, { default_fee_rate_ashare_sell: 0.0007 })
  })
})
