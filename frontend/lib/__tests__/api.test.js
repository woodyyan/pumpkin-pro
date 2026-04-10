// ── Pure function tests for api.js (isNetworkError) ──
// Uses Node 20+ built-in test runner (node --test)

import { describe, it } from 'node:test'
import assert from 'node:assert/strict'

// Exact copy of isNetworkError from lib/api.js
function isNetworkError(error) {
  if (!error) return false
  // status === 0 means request never completed (CORS, abort, network down)
  if (error.status === 0 || error.status === undefined) return true
  // Standard HTTP status codes for server-side / proxy issues
  const s = Number(error.status)
  if (s >= 500 && s < 600) return true   // 5xx = server error
  if (s === 502 || s === 503 || s === 504) return true // gateway/proxy errors
  return false
}

describe('isNetworkError', () => {
  it('returns false for null/undefined error', () => {
    assert.equal(isNetworkError(null), false)
    assert.equal(isNetworkError(undefined), false)
  })

  it('returns true for status === 0 (CORS / abort)', () => {
    assert.equal(isNetworkError({ status: 0 }), true)
  })

  it('returns true for undefined status', () => {
    assert.equal(isNetworkError({}), true)
    assert.equal(isNetworkError({ message: 'fail' }), true)
  })

  it('returns true for all 5xx errors', () => {
    assert.equal(isNetworkError({ status: 500 }), true)
    assert.equal(isNetworkError({ status: 502 }), true)
    assert.equal(isNetworkError({ status: 503 }), true)
    assert.equal(isNetworkError({ status: 504 }), true)
    assert.equal(isNetworkError({ status: 599 }), true)
  })

  it('returns false for 4xx client errors', () => {
    assert.equal(isNetworkError({ status: 400 }), false)
    assert.equal(isNetworkError({ status: 401 }), false)
    assert.equal(isNetworkError({ status: 403 }), false)
    assert.equal(isNetworkError({ status: 404 }), false)
    assert.equal(isNetworkError({ status: 422 }), false)
  })

  it('returns false for success codes', () => {
    assert.equal(isNetworkError({ status: 200 }), false)
    assert.equal(isNetworkError({ status: 201 }), false)
    assert.equal(isNetworkError({ status: 204 }), false)
    assert.equal(isNetworkError({ status: 301 }), false)
  })

  it('handles string status coerced to number', () => {
    // Note: status check uses strict equality ('0' !== 0), so string '0'
    // falls through to Number() path where 0 doesn't match any network error range
    assert.equal(isNetworkError({ status: '0' }), false)  // strict check fails
    assert.equal(isNetworkError({ status: '500' }), true)
    assert.equal(isNetworkError({ status: '200' }), false)
  })

  it('returns false for non-standard statuses outside 5xx', () => {
    const result = isNetworkError({ status: 'abc' })
    assert.equal(typeof result, 'boolean')
    // Number("abc") = NaN; NaN >= 500 is false, NaN < 600 is false
    assert.equal(result, false)
  })
})
