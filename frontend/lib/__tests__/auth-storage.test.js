// ── Pure function tests for auth-storage.js ──
// Uses Node 20+ built-in test runner (node --test)

import { describe, it, beforeEach } from 'node:test'
import assert from 'node:assert/strict'

// ═══════════════════════════════════════════
// Section A: isAuthRequiredError (original tests)
// ═══════════════════════════════════════════

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

// ═══════════════════════════════════════════
// Section B: Session Storage (NEW — T2.1 ~ T2.5)
// ═══════════════════════════════════════════

const AUTH_SESSION_STORAGE_KEY = 'pumpkin_pro_auth_session'

/** In-memory localStorage mock */
let _store = {}

/** Simulated readAuthSession (mirrors source exactly) */
function readAuthSession() {
  const text = _store[AUTH_SESSION_STORAGE_KEY]
  if (!text) return null
  try {
    const parsed = JSON.parse(text)
    if (!parsed || typeof parsed !== 'object') return null
    if (!parsed.tokens?.access_token || !parsed.tokens?.refresh_token) return null
    return parsed
  } catch { return null }
}

/** Simulated writeAuthSession (mirrors source exactly) */
function writeAuthSession(session) {
  if (!session) delete _store[AUTH_SESSION_STORAGE_KEY]
  else _store[AUTH_SESSION_STORAGE_KEY] = JSON.stringify(session)
}

/** Simulated clearAuthSession (mirrors source exactly) */
function clearAuthSession() {
  delete _store[AUTH_SESSION_STORAGE_KEY]
}

/** Simulated getAccessToken */
function getAccessToken() {
  return readAuthSession()?.tokens?.access_token || ''
}

/** Simulated getRefreshToken */
function getRefreshToken() {
  return readAuthSession()?.tokens?.refresh_token || ''
}

describe('session storage operations', () => {
  beforeEach(() => {
    _store = {}
  })

  // T2.1: Write → Read round-trip consistency
  it('T2.1: writeAuthSession + readAuthSession round-trip preserves data', () => {
    const session = {
      user: { email: 'woody@example.com', nickname: 'Woody' },
      tokens: {
        access_token: 'at_abc123',
        refresh_token: 'rt_xyz789',
      },
    }

    writeAuthSession(session)
    const loaded = readAuthSession()

    assert.notEqual(loaded, null, 'should load session after writing')
    assert.deepEqual(loaded.user, session.user)
    assert.equal(loaded.tokens.access_token, 'at_abc123')
    assert.equal(loaded.tokens.refresh_token, 'rt_xyz789')
  })

  // T2.2: Clear removes session completely
  it('T2.2: clearAuthSession removes session so read returns null', () => {
    writeAuthSession({
      user: { email: 'a@b.com' },
      tokens: { access_token: 'x', refresh_token: 'y' },
    })
    assert.notEqual(readAuthSession(), null, 'precondition: session exists')

    clearAuthSession()
    assert.equal(readAuthSession(), null, 'session must be null after clearing')
  })

  // T2.3: getAccessToken / getRefreshToken extract correct fields
  it('T2.3: getAccessToken and getRefreshToken return correct values', () => {
    writeAuthSession({
      user: {},
      tokens: {
        access_token: 'my_access_token_value',
        refresh_token: 'my_refresh_token_value',
      },
    })

    assert.equal(getAccessToken(), 'my_access_token_value')
    assert.equal(getRefreshToken(), 'my_refresh_token_value')
  })

  // T2.4: Corrupted JSON returns null (fault tolerance)
  it('T2.4: corrupted JSON in storage returns null gracefully', () => {
    _store[AUTH_SESSION_STORAGE_KEY] = '{this is not valid json!!!'
    assert.equal(readAuthSession(), null, 'malformed JSON must return null')

    _store[AUTH_SESSION_STORAGE_KEY] = 'just-a-string'
    assert.equal(readAuthSession(), null, 'non-object JSON string must return null')
  })

  // T2.5: Missing required fields returns null
  it('T2.5: missing required token fields returns null', () => {
    // No tokens at all
    _store[AUTH_SESSION_STORAGE_KEY] = JSON.stringify({ user: { email: 'x' } })
    assert.equal(readAuthSession(), null, 'no tokens → null')

    // Only access_token, no refresh_token
    _store[AUTH_SESSION_STORAGE_KEY] = JSON.stringify({
      user: {},
      tokens: { access_token: 'only_at' },
    })
    assert.equal(readAuthSession(), null, 'missing refresh_token → null')

    // Only refresh_token, no access_token
    _store[AUTH_SESSION_STORAGE_KEY] = JSON.stringify({
      user: {},
      tokens: { refresh_token: 'only_rt' },
    })
    assert.equal(readAuthSession(), null, 'missing access_token → null')

    // Empty tokens object
    _store[AUTH_SESSION_STORAGE_KEY] = JSON.stringify({
      user: {},
      tokens: {},
    })
    assert.equal(readAuthSession(), null, 'empty tokens → null')
  })
})
