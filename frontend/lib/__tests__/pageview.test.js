import { describe, it, beforeEach } from 'node:test'
import assert from 'node:assert/strict'

import { buildPageViewHeaders } from '../pageview.js'
import { writeAuthSession, clearAuthSession } from '../auth-storage.js'

describe('buildPageViewHeaders', () => {
  beforeEach(() => {
    global.window = {
      localStorage: createStorage(),
    }
    clearAuthSession()
  })

  it('includes content type for anonymous visitors', () => {
    const headers = buildPageViewHeaders()
    assert.deepEqual(headers, { 'Content-Type': 'application/json' })
  })

  it('includes bearer token for logged-in users so page views can be linked to user_id', () => {
    writeAuthSession({
      user: { id: 'u1', email: 'alice@example.com' },
      tokens: { access_token: 'acc_123', refresh_token: 'ref_456' },
    })

    const headers = buildPageViewHeaders()
    assert.equal(headers['Content-Type'], 'application/json')
    assert.equal(headers.Authorization, 'Bearer acc_123')
  })
})

function createStorage() {
  const store = new Map()
  return {
    getItem(key) {
      return store.has(key) ? store.get(key) : null
    },
    setItem(key, value) {
      store.set(key, String(value))
    },
    removeItem(key) {
      store.delete(key)
    },
  }
}
