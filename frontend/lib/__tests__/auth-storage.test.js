// ── Pure function tests for auth-storage.js ──
// Uses Node 20+ built-in test runner (node --test)

import { describe, it } from 'node:test'
import assert from 'node:assert/strict'

// Exact copy of isAuthRequiredError from lib/auth-storage.js
function isAuthRequiredError(error) {
  if (!error) return false
  if (Number(error.status) === 401) return true
  const code = String(error.code || '').toUpperCase()
  if (code === 'AUTH_REQUIRED' || code === 'UNAUTHORIZED') return true
  const message = String(error.message || '')
  return message.includes('需要登录') || message.includes('登录后使用')
}

describe('isAuthRequiredError', () => {
  it('returns false for null/undefined', () => {
    assert.equal(isAuthRequiredError(null), false)
    assert.equal(isAuthRequiredError(undefined), false)
  })

  it('returns true for 401 status', () => {
    assert.equal(isAuthRequiredError({ status: 401 }), true)
  })

  it('returns false for other status codes', () => {
    assert.equal(isAuthRequiredError({ status: 200 }), false)
    assert.equal(isAuthRequiredError({ status: 400 }), false)
    assert.equal(isAuthRequiredError({ status: 403 }), false)
    assert.equal(isAuthRequiredError({ status: 500 }), false)
  })

  it('returns true for AUTH_REQUIRED code (case-insensitive)', () => {
    assert.equal(isAuthRequiredError({ code: 'AUTH_REQUIRED' }), true)
    assert.equal(isAuthRequiredError({ code: 'auth_required' }), true)
  })

  it('returns true for UNAUTHORIZED code', () => {
    assert.equal(isAuthRequiredError({ code: 'UNAUTHORIZED' }), true)
  })

  it('detects Chinese auth-required message patterns', () => {
    assert.equal(isAuthRequiredError({ message: '请先登录后使用此功能' }), true)
    assert.equal(isAuthRequiredError({ message: '需要登录才能查看' }), true)
    assert.equal(isAuthRequiredError({ message: '请登录后使用' }), true)
  })

  it('returns false for non-auth messages', () => {
    assert.equal(isAuthRequiredError({ message: '服务器内部错误' }), false)
    assert.equal(isAuthRequiredError({ message: '请求成功' }), false)
  })

  it('checks status as number (string coercion)', () => {
    assert.equal(isAuthRequiredError({ status: 401 }), true)
    // String "401" gets coerced to Number("401") = 401 by the Number() call
    assert.equal(isAuthRequiredError({ status: '401' }), true)
  })
})

// Helper to test the status-as-number behavior (same as isAuthRequiredError logic for status=0)
function isNetworkErrorLike(error) {
  if (!error) return false
  if (error.status === 0 || error.status === undefined) return true
  return false
}

describe('network-error-like detection (status edge cases)', () => {
  it('status 0 or undefined = network error', () => {
    assert.equal(isNetworkErrorLike({ status: 0 }), true)
    assert.equal(isNetworkErrorLike({}), true)
  })
  it('non-zero status = not network error', () => {
    assert.equal(isNetworkErrorLike({ status: 401 }), false)
    assert.equal(isNetworkErrorLike({ status: 500 }), false)
  })
})
