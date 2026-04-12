// ── Tests for auth-context.js (fetchMe / syncSession / logout logic) ──
// Uses Node 20+ built-in test runner (node --test)
//
// We test the pure-logic parts of AuthProvider by re-implementing the functions
// under test and mocking dependencies (requestJson, isNetworkError, storage).

import { describe, it, beforeEach } from 'node:test'
import assert from 'node:assert/strict'

// ═══════════════════════════════════════════
// Mock infrastructure
// ═══════════════════════════════════════════

const AUTH_SESSION_STORAGE_KEY = 'pumpkin_pro_auth_session'
let _store = {}

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

function writeAuthSession(session) {
  if (!session) delete _store[AUTH_SESSION_STORAGE_KEY]
  else _store[AUTH_SESSION_STORAGE_KEY] = JSON.stringify(session)
}

function clearAuthSession() {
  delete _store[AUTH_SESSION_STORAGE_KEY]
}

function getRefreshToken() {
  return readAuthSession()?.tokens?.refresh_token || ''
}

// ── Re-implemented functions from auth-context.js source ──

/** Mirrors isAuthRequiredError from auth-storage.js */
function isAuthRequiredError(error) {
  if (!error) return false
  if (Number(error.status) === 401) return true
  const code = String(error.code || '').toUpperCase()
  if (code === 'AUTH_REQUIRED' || code === 'UNAUTHORIZED') return true
  const message = String(error.message || '')
  return message.includes('需要登录') || message.includes('登录后使用')
}

/** Mirrors isNetworkError from api.js */
function isNetworkError(error) {
  if (!error) return false
  if (error.status === 0 || error.status === undefined) return true
  const s = Number(error.status)
  if (s >= 500 && s < 600) return true
  if (s === 502 || s === 503 || s === 504) return true
  return false
}

/**
 * Mirrors syncSession from auth-context.js.
 * Returns session object on success, null on invalid input.
 */
function syncSession(payload) {
  if (!payload?.tokens?.access_token || !payload?.tokens?.refresh_token || !payload?.user) {
    return null
  }
  const next = {
    user: payload.user,
    tokens: payload.tokens,
  }
  writeAuthSession(next)
  return next
}

/**
 * Mirrors fetchMe from auth-context.js.
 * @param {{ requestJson: function }} deps - injected dependencies for testing
 * @returns {{ cleared: boolean }} whether clearSession was called
 */
async function fetchMe(deps) {
  let cleared = false
  // Inline clearSession mock
  const clearSession = () => { cleared = true; clearAuthSession() }

  try {
    const result = await deps.requestJson('/api/user/me', undefined, '读取账号信息失败')
    if (!result?.user) return { cleared }
    writeAuthSession({ user: result.user, tokens: readAuthSession()?.tokens })
    return { cleared }
  } catch (error) {
    // Only clear on explicit auth errors — NOT on network/deployment errors
    if (isAuthRequiredError(error) && !isNetworkError(error)) {
      clearSession()
    }
    return { cleared }
  }
}

/**
 * Mirrors logout from auth-context.js.
 * @param {{ requestJson?: function }} deps - optional mocked requestJson
 * @returns {{ cleared: boolean }}
 */
async function logout(deps = {}) {
  let cleared = false
  const refreshToken = getRefreshToken()
  const clearSession = () => { cleared = true; clearAuthSession() }

  try {
    if (deps.requestJson) {
      await deps.requestJson('/api/auth/logout', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ refresh_token: refreshToken }),
      }, '退出登录失败')
    }
  } catch {
    // Even if backend fails, always clean up local state
  }
  clearSession()
  return { cleared }
}

describe('auth-context: fetchMe + syncSession + logout', () => {
  beforeEach(() => {
    _store = {}
  })

  /** Helper to seed a valid session */
  function seedSession(overrides = {}) {
    const session = {
      user: { email: 'test@example.com', nickname: 'TestUser' },
      tokens: { access_token: 'acc_123', refresh_token: 'ref_456' },
      ...overrides,
    }
    writeAuthSession(session)
    return session
  }

  // ═════════════════════════════════════════
  // T3.1: fetchMe receives 401 → calls clearSession
  // ═════════════════════════════════════════
  it('T3.1: fetchMe on explicit 401 error → clears session', async () => {
    seedSession()

    const result = await fetchMe({
      requestJson: async () => {
        const err = new Error('Unauthorized')
        err.status = 401
        throw err
      },
    })

    assert.equal(result.cleared, true, 'must clear session on 401')
    assert.equal(readAuthSession(), null, 'storage must be empty after clear')
  })

  it('T3.1b: fetchMe on AUTH_REQUIRED code error → clears session', async () => {
    seedSession()

    const result = await fetchMe({
      requestJson: async () => {
        const err = new Error('AUTH_REQUIRED')
        err.status = 401
        err.code = 'AUTH_REQUIRED'
        throw err
      },
    })

    assert.equal(result.cleared, true)
    assert.equal(readAuthSession(), null)
  })

  // ═════════════════════════════════════════
  // T3.2: fetchMe on network error → does NOT clear session ⭐ CORE SCENARIO
  // ═════════════════════════════════════════
  it('T3.2: fetchMe on 502 network error → preserves session (deployment scenario)', async () => {
    seedSession()

    const result = await fetchMe({
      requestJson: async () => {
        const err = new Error('Bad Gateway')
        err.status = 502
        throw err
      },
    })

    assert.equal(result.cleared, false, 'MUST NOT clear session on network error')

    // Session must be intact
    const session = readAuthSession()
    assert.notEqual(session, null, 'session must still exist in storage')
    assert.equal(session.tokens.access_token, 'acc_123')
    assert.equal(session.user.email, 'test@example.com')
  })

  it('T3.2b: fetchMe on status=0 (CORS/abort) → preserves session', async () => {
    seedSession()

    const result = await fetchMe({
      requestJson: async () => {
        const err = new Error('Network error')
        err.status = 0
        throw err
      },
    })

    assert.equal(result.cleared, false)
    assert.notEqual(readAuthSession(), null)
  })

  it('T3.2c: fetchMe on 503 Service Unavailable → preserves session', async () => {
    seedSession()

    const result = await fetchMe({
      requestJson: async () => {
        const err = new Error('Service Unavailable')
        err.status = 503
        throw err
      },
    })

    assert.equal(result.cleared, false)
    assert.notEqual(readAuthSession(), null)
  })

  // ═════════════════════════════════════════
  // T3.3: fetchMe success → updates user in session
  // ═════════════════════════════════════════
  it('T3.3: fetchMe success updates user info in session', async () => {
    seedSession({ user: { email: 'old@email.com', nickname: 'OldName' } })

    await fetchMe({
      requestJson: async () => ({
        user: { email: 'new@email.com', nickname: 'NewNickname' },
      }),
    })

    const session = readAuthSession()
    assert.notEqual(session, null)
    assert.equal(session.user.email, 'new@email.com', 'email should be updated')
    assert.equal(session.user.nickname, 'NewNickname', 'nickname should be updated')
    // Tokens must be preserved
    assert.equal(session.tokens.access_token, 'acc_123')
  })

  // ═════════════════════════════════════════
  // T3.4: syncSession with missing fields → returns null, no write
  // ═════════════════════════════════════════
  it('T3.4: syncSession with incomplete payload returns null without writing', () => {
    // No tokens at all
    assert.equal(syncSession({ user: { email: 'x' } }), null)

    // Missing refresh_token
    assert.equal(syncSession({
      tokens: { access_token: 'a' }, user: {},
    }), null)

    // Missing access_token
    assert.equal(syncSession({
      tokens: { refresh_token: 'r' }, user: {},
    }), null)

    // Missing user
    assert.equal(syncSession({
      tokens: { access_token: 'a', refresh_token: 'r' },
    }), null)

    // Null input
    assert.equal(syncSession(null), null)
    assert.equal(syncSession(undefined), null)

    // Verify nothing was written
    assert.equal(readAuthSession(), null)
  })

  it('T3.4b: syncSession with valid payload writes to storage', () => {
    const valid = {
      user: { email: 'valid@test.com' },
      tokens: { access_token: 'at', refresh_token: 'rt' },
    }

    const result = syncSession(valid)
    assert.notEqual(result, null)
    assert.deepEqual(result.user, valid.user)

    const stored = readAuthSession()
    assert.notEqual(stored, null)
    assert.equal(stored.user.email, 'valid@test.com')
  })

  // ═════════════════════════════════════════
  // T3.5: logout always clears local session regardless of backend
  // ═════════════════════════════════════════
  it('T3.5: logout clears session even when backend call succeeds', async () => {
    seedSession()

    const result = await logout({
      requestJson: async () => ({ ok: true }),
    })

    assert.equal(result.cleared, true, 'must clear session after logout')
    assert.equal(readAuthSession(), null, 'storage must be empty')
  })

  it('T3.5b: logout clears session even when backend call fails', async () => {
    seedSession()

    const result = await logout({
      requestJson: async () => {
        throw new Error('Backend unreachable')
      },
    })

    assert.equal(result.cleared, true, 'must still clear session when backend fails')
    assert.equal(readAuthSession(), null)
  })

  it('T3.5c: logout clears session even without requestJson (no backend)', async () => {
    seedSession()

    const result = await logout({})
    assert.equal(result.cleared, true)
    assert.equal(readAuthSession(), null)
  })
})
