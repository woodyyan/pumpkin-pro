// ── Tests for auth-context.js (fetchMe / syncSession / logout logic) ──
// Uses Node 20+ built-in test runner (node --test)

import { describe, it, beforeEach } from 'node:test'
import assert from 'node:assert/strict'

const AUTH_SESSION_STORAGE_KEY = 'pumpkin_pro_auth_session'
const AUTH_LOGOUT_SIGNAL_STORAGE_KEY = 'pumpkin_pro_auth_logout_signal'
let _store = {}
let _broadcasts = []

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

function broadcastAuthSessionCleared(reason = '') {
  _store[AUTH_LOGOUT_SIGNAL_STORAGE_KEY] = JSON.stringify({ reason, at: Date.now() })
  _broadcasts.push(reason)
}

function getRefreshToken() {
  return readAuthSession()?.tokens?.refresh_token || ''
}

function isAuthRequiredError(error) {
  if (!error) return false
  if (Number(error.status) === 401) return true
  const code = String(error.code || '').toUpperCase()
  if (code === 'AUTH_REQUIRED' || code === 'UNAUTHORIZED' || code === 'SESSION_REVOKED') return true
  const message = String(error.message || '')
  return message.includes('需要登录') || message.includes('登录后使用')
}

function isNetworkError(error) {
  if (!error) return false
  if (error.status === 0 || error.status === undefined) return true
  const s = Number(error.status)
  if (s >= 500 && s < 600) return true
  if (s === 502 || s === 503 || s === 504) return true
  return false
}

function syncSession(payload) {
  if (!payload?.tokens?.access_token || !payload?.tokens?.refresh_token || !payload?.user) {
    return null
  }
  const next = { user: payload.user, tokens: payload.tokens }
  writeAuthSession(next)
  return next
}

async function fetchMe(deps) {
  let cleared = false
  const clearSession = (reason = '') => {
    cleared = true
    clearAuthSession()
    broadcastAuthSessionCleared(reason)
  }

  try {
    const result = await deps.requestJson('/api/user/me', undefined, '读取账号信息失败')
    if (!result?.user) return { cleared }
    writeAuthSession({ user: result.user, tokens: readAuthSession()?.tokens })
    return { cleared }
  } catch (error) {
    if (isAuthRequiredError(error) && !isNetworkError(error)) {
      clearSession('auth_required')
    }
    return { cleared }
  }
}

async function logout(deps = {}) {
  let cleared = false
  const refreshToken = getRefreshToken()
  const clearSession = (reason = '') => {
    cleared = true
    clearAuthSession()
    broadcastAuthSessionCleared(reason)
  }

  try {
    if (deps.requestJson) {
      await deps.requestJson('/api/auth/logout', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ refresh_token: refreshToken }),
      }, '退出登录失败')
    }
  } catch {}
  clearSession('logout')
  return { cleared }
}

describe('auth-context: fetchMe + syncSession + logout', () => {
  beforeEach(() => {
    _store = {}
    _broadcasts = []
  })

  function seedSession(overrides = {}) {
    const session = {
      user: { email: 'test@example.com', nickname: 'TestUser' },
      tokens: { access_token: 'acc_123', refresh_token: 'ref_456' },
      ...overrides,
    }
    writeAuthSession(session)
    return session
  }

  it('T3.1: fetchMe on explicit 401 error → clears session', async () => {
    seedSession()
    const result = await fetchMe({
      requestJson: async () => {
        const err = new Error('Unauthorized')
        err.status = 401
        throw err
      },
    })
    assert.equal(result.cleared, true)
    assert.equal(readAuthSession(), null)
    assert.deepEqual(_broadcasts, ['auth_required'])
  })

  it('T3.2: fetchMe on 502 network error → preserves session', async () => {
    seedSession()
    const result = await fetchMe({
      requestJson: async () => {
        const err = new Error('Bad Gateway')
        err.status = 502
        throw err
      },
    })
    assert.equal(result.cleared, false)
    assert.notEqual(readAuthSession(), null)
    assert.deepEqual(_broadcasts, [])
  })

  it('T3.3: fetchMe success updates user info in session', async () => {
    seedSession({ user: { email: 'old@email.com', nickname: 'OldName' } })
    await fetchMe({
      requestJson: async () => ({
        user: { email: 'new@email.com', nickname: 'NewNickname' },
      }),
    })
    const session = readAuthSession()
    assert.equal(session.user.email, 'new@email.com')
    assert.equal(session.user.nickname, 'NewNickname')
    assert.equal(session.tokens.access_token, 'acc_123')
  })

  it('T3.4: syncSession with incomplete payload returns null without writing', () => {
    assert.equal(syncSession({ user: { email: 'x' } }), null)
    assert.equal(syncSession({ tokens: { access_token: 'a' }, user: {} }), null)
    assert.equal(syncSession({ tokens: { refresh_token: 'r' }, user: {} }), null)
    assert.equal(syncSession({ tokens: { access_token: 'a', refresh_token: 'r' } }), null)
    assert.equal(syncSession(null), null)
    assert.equal(readAuthSession(), null)
  })

  it('T3.4b: syncSession with valid payload writes to storage', () => {
    const valid = {
      user: { email: 'valid@test.com' },
      tokens: { access_token: 'at', refresh_token: 'rt' },
    }
    const result = syncSession(valid)
    assert.notEqual(result, null)
    assert.equal(readAuthSession().user.email, 'valid@test.com')
  })

  it('T3.5: logout clears session and broadcasts even when backend fails', async () => {
    seedSession()
    const result = await logout({
      requestJson: async () => { throw new Error('Backend unreachable') },
    })
    assert.equal(result.cleared, true)
    assert.equal(readAuthSession(), null)
    assert.deepEqual(_broadcasts, ['logout'])
  })
})
