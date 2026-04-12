// ── Pure function tests for api.js ──
// Uses Node 20+ built-in test runner (node --test)

import { describe, it, beforeEach } from 'node:test'
import assert from 'node:assert/strict'

// ═══════════════════════════════════════════
// Section A: isNetworkError (original tests)
// ═══════════════════════════════════════════

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

// ═══════════════════════════════════════════
// Section B: _fetchOnce + tryRefreshToken (NEW — deployment-resilience tests)
// ═══════════════════════════════════════════
//
// These test the core 401→refresh→retry logic by re-implementing the
// functions under test (same copy-paste pattern used in other test files).
// We mock `fetch` and localStorage to simulate various scenarios.

describe('_fetchOnce + tryRefreshToken', () => {

  // ── Mock infrastructure ──

  let _mockStorage = {}
  let _fetchMock = null   // fn(url, init) => Response-like
  let _refreshPromise = null

  /** Create a fake fetch Response */
  function makeResponse(status, body, headers = {}) {
    const textBody = typeof body === 'string' ? body : JSON.stringify(body)
    return {
      ok: status >= 200 && status < 300,
      status,
      headers: new Map(Object.entries(headers)),
      json: async () => (typeof body === 'string' ? JSON.parse(textBody) : body),
      text: async () => textBody,
    }
  }

  /** Simulated auth-storage functions */
  const STORAGE_KEY = 'pumpkin_pro_auth_session'

  function readAuthSession() {
    const text = _mockStorage[STORAGE_KEY]
    if (!text) return null
    try {
      const parsed = JSON.parse(text)
      if (!parsed || typeof parsed !== 'object') return null
      if (!parsed.tokens?.access_token || !parsed.tokens?.refresh_token) return null
      return parsed
    } catch { return null }
  }

  function writeAuthSession(session) {
    if (!session) delete _mockStorage[STORAGE_KEY]
    else _mockStorage[STORAGE_KEY] = JSON.stringify(session)
  }

  function clearAuthSession() {
    delete _mockStorage[STORAGE_KEY]
  }

  function getAccessToken() {
    return readAuthSession()?.tokens?.access_token || ''
  }

  function getRefreshToken() {
    return readAuthSession()?.tokens?.refresh_token || ''
  }

  /** Re-implementation of tryRefreshToken (mirrors source exactly) */
  async function tryRefreshToken() {
    if (_refreshPromise) return _refreshPromise

    _refreshPromise = (async () => {
      const refreshToken = getRefreshToken()
      if (!refreshToken) {
        clearAuthSession()
        return 'auth_failed'
      }

      try {
        const res = await _fetchMock('/api/auth/refresh', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ refresh_token: refreshToken }),
        })

        if (!res.ok) {
          clearAuthSession()
          return 'auth_failed'
        }

        const data = await res.json()
        if (!data?.tokens?.access_token || !data?.tokens?.refresh_token) {
          clearAuthSession()
          return 'auth_failed'
        }

        const existing = readAuthSession() || {}
        const next = {
          ...existing,
          tokens: data.tokens,
          user: data.user || existing.user,
        }
        writeAuthSession(next)
        return 'ok'
      } catch {
        return 'network_failed'
      } finally {
        _refreshPromise = null
      }
    })()

    return _refreshPromise
  }

  /** Re-implementation of _fetchOnce (mirrors source logic) */
  async function readApiResponse(response) {
    const text = await response.text()
    if (!text) return null
    try {
      return JSON.parse(text)
    } catch {
      return { detail: text }
    }
  }

  async function _fetchOnce(input, init = {}, fallbackMessage = '请求失败') {
    const headers = new Headers(init?.headers || {})
    if (!headers.has('accept')) {
      headers.set('Accept', 'application/json')
    }

    const accessToken = getAccessToken()
    if (accessToken && !headers.has('authorization')) {
      headers.set('Authorization', `Bearer ${accessToken}`)
    }

    const response = await _fetchMock(input, {
      ...init,
      headers,
    })
    const data = await readApiResponse(response)

    if (response.ok) {
      return data
    }

    // On 401 → attempt silent token refresh, then retry once
    if (response.status === 401) {
      const result = await tryRefreshToken()
      if (result === 'ok') {
        // Retry original request with fresh token
        const retryHeaders = new Headers(init?.headers || {})
        if (!retryHeaders.has('accept')) retryHeaders.set('Accept', 'application/json')
        const newToken = getAccessToken()
        if (newToken) retryHeaders.set('Authorization', `Bearer ${newToken}`)

        const retryResponse = await _fetchMock(input, { ...init, headers: retryHeaders })
        const retryData = await readApiResponse(retryResponse)

        if (retryResponse.ok) return retryData
        if (retryResponse.status === 401) {
          clearAuthSession()
          throw buildApiError(retryResponse, retryData, fallbackMessage)
        }
        throw buildApiError(retryResponse, retryData, fallbackMessage)
      }

      // Network failure during deployment
      if (result === 'network_failed') {
        throw { status: 0, message: '网络暂时不可用，请稍后重试', transient: true }
      }

      // auth_failed — server explicitly rejected credentials
      clearAuthSession()
    }

    throw buildApiError(response, data, fallbackMessage)
  }

  function buildApiError(response, responseData, fallbackText) {
    const error = new Error(
      (responseData && typeof responseData === 'object' && !Array.isArray(responseData) && 'detail' in responseData)
        ? (typeof responseData.detail === 'string' ? responseData.detail : String(responseData.detail))
        : fallbackText
    )
    error.status = response?.status || 0
    error.responseData = responseData
    return error
  }

  // ── Reset before each test ──
  beforeEach(() => {
    _mockStorage = {}
    _fetchMock = null
    _refreshPromise = null
  })

  // Helper: seed a valid session into mock storage
  function seedSession(overrides = {}) {
    const session = {
      user: { email: 'test@example.com', nickname: 'TestUser' },
      tokens: {
        access_token: 'acc_old',
        refresh_token: 'ref_old',
      },
      ...overrides,
    }
    writeAuthSession(session)
    return session
  }

  // ═════════════════════════════════════════
  // T1.1: 401 → refresh succeeds → retry returns 200
  // ═════════════════════════════════════════
  it('T1.1: 401 → refresh succeeds → retry returns 200 (happy path)', async () => {
    seedSession()
    let callCount = 0

    _fetchMock = (url, init) => {
      callCount++
      if (url === '/api/auth/refresh') {
        return makeResponse(200, {
          tokens: { access_token: 'acc_new', refresh_token: 'ref_new' },
          user: { email: 'test@example.com', nickname: 'TestUser' },
        })
      }
      // First call to target API returns 401, second returns 200 after refresh
      if (callCount === 1) return makeResponse(401, { detail: 'Unauthorized' })
      return makeResponse(200, { data: 'success_after_refresh' })
    }

    const result = await _fetchOnce('/api/some-resource')
    assert.deepEqual(result, { data: 'success_after_refresh' })

    // Verify new token was stored
    const session = readAuthSession()
    assert.equal(session.tokens.access_token, 'acc_new')
    assert.equal(session.tokens.refresh_token, 'ref_new')
  })

  // ═════════════════════════════════════════
  // T1.2: 401 → refresh succeeds → retry still 401 → clear session
  // ═════════════════════════════════════════
  it('T1.2: 401 → refresh ok but retry still 401 → clears session', async () => {
    seedSession()
    let callCount = 0

    _fetchMock = (url) => {
      callCount++
      if (url === '/api/auth/refresh') {
        return makeResponse(200, {
          tokens: { access_token: 'acc_new', refresh_token: 'ref_new' },
        })
      }
      // Both original and retry return 401
      return makeResponse(401, { detail: 'Forbidden' })
    }

    await assert.rejects(async () => {
      await _fetchOnce('/api/resource')
    }, /Forbidden/)

    // Session must be cleared
    assert.equal(readAuthSession(), null)
  })

  // ═════════════════════════════════════════
  // T1.3: 401 → refresh rejected by server (401/400) → clear session
  // ═════════════════════════════════════════
  it('T1.3: 401 → server rejects refresh token → clears session (auth_failed)', async () => {
    seedSession()

    _fetchMock = (url) => {
      if (url === '/api/auth/refresh') {
        // Server says the refresh token is invalid/expired/revoked
        return makeResponse(401, { detail: 'Refresh token has been revoked' })
      }
      return makeResponse(401, { detail: 'Unauthorized' })
    }

    await assert.rejects(async () => {
      await _fetchOnce('/api/resource')
    }, /Unauthorized/)

    // Session must be cleared on auth failure
    assert.equal(readAuthSession(), null)
  })

  it('T1.3b: 401 → refresh returns 400 bad request → clears session', async () => {
    seedSession()

    _fetchMock = (url) => {
      if (url === '/api/auth/refresh') {
        return makeResponse(400, { detail: 'Invalid token format' })
      }
      return makeResponse(401, { detail: 'Unauthorized' })
    }

    await assert.rejects(async () => {
      await _fetchOnce('/api/resource')
    })

    assert.equal(readAuthSession(), null)
  })

  // ═════════════════════════════════════════
  // T1.4: 401 → refresh throws network error → PRESERVE session ⭐ CORE SCENARIO
  // ═════════════════════════════════════════
  it('T1.4: 401 → refresh network error → preserves session (deployment scenario)', async () => {
    seedSession()

    _fetchMock = (url) => {
      if (url === '/api/auth/refresh') {
        // Simulate 502 Bad Gateway or network unreachable during deployment
        throw new Error('fetch failed: ECONNREFUSED')
      }
      return makeResponse(401, { detail: 'Unauthorized' })
    }

    let caughtErr
    try {
      await _fetchOnce('/api/resource')
    } catch (e) {
      caughtErr = e
    }

    // Must throw a transient error (not clear session)
    assert.ok(caughtErr, 'should throw an error')
    assert.equal(caughtErr.transient, true, 'must be marked as transient')
    assert.equal(caughtErr.status, 0, 'transient errors have status 0')

    // ★ THE KEY ASSERTION: session is still intact
    const session = readAuthSession()
    assert.notEqual(session, null, 'session MUST be preserved during network errors')
    assert.equal(session.tokens.access_token, 'acc_old', 'old access token still present')
    assert.equal(session.tokens.refresh_token, 'ref_old', 'old refresh token still present')
  })

  it('T1.4b: 401 → refresh times out → preserves session', async () => {
    seedSession()

    _fetchMock = (url) => {
      if (url === '/api/auth/refresh') {
        throw new Error('AbortError: The operation was aborted')
      }
      return makeResponse(401, {})
    }

    let caughtErr
    try { await _fetchOnce('/api/resource') } catch (e) { caughtErr = e }

    assert.equal(caughtErr.transient, true)
    assert.notEqual(readAuthSession(), null, 'session preserved on timeout')
  })

  // ═════════════════════════════════════════
  // T1.5: 401 → no refresh token → clear session
  // ═════════════════════════════════════════
  it('T1.5: 401 → no refresh token available → clears session', async () => {
    // Seed session with only access token (no refresh token — abnormal state)
    _mockStorage[STORAGE_KEY] = JSON.stringify({
      user: { email: 'test@example.com' },
      tokens: { access_token: 'acc_only' }, // no refresh_token
    })

    _fetchMock = () => makeResponse(401, { detail: 'Unauthorized' })

    await assert.rejects(async () => {
      await _fetchOnce('/api/resource')
    })

    assert.equal(readAuthSession(), null, 'session cleared when no refresh token')
  })

  // ═════════════════════════════════════════
  // T1.6: Concurrent 401s → only one refresh call
  // ═════════════════════════════════════════
  it('T1.6: concurrent 401 requests trigger only one refresh call (singleton)', async () => {
    seedSession()
    let refreshCallCount = 0
    const callCountPerUrl = new Map()

    _fetchMock = (url) => {
      if (url === '/api/auth/refresh') {
        refreshCallCount++
        return makeResponse(200, {
          tokens: { access_token: 'acc_shared', refresh_token: 'ref_shared' },
        })
      }
      // First call to each resource URL = 401; retry after refresh = 200
      const count = (callCountPerUrl.get(url) || 0) + 1
      callCountPerUrl.set(url, count)
      if (count === 1) {
        return makeResponse(401, { detail: 'Unauthorized' })
      }
      return makeResponse(200, { data: `ok:${url}` })
    }

    // Fire two simultaneous 401-handling requests
    const [r1, r2] = await Promise.allSettled([
      _fetchOnce('/api/resource-a'),
      _fetchOnce('/api/resource-b'),
    ])

    // Only ONE refresh call should have been made
    assert.equal(refreshCallCount, 1, 'must only refresh once for concurrent 401s')

    // Both should eventually succeed (retry after shared refresh)
    assert.equal(r1.status, 'fulfilled', 'first request should succeed after shared refresh')
    assert.equal(r2.status, 'fulfilled', 'second request should succeed after shared refresh')
  })

  // ═════════════════════════════════════════
  // T1.7: Bearer Token auto-injection when session exists
  // ═════════════════════════════════════════
  it('T1.7: auto-injects Bearer token when user has session', async () => {
    seedSession()
    let capturedInit = null

    _fetchMock = (url, init) => {
      capturedInit = init
      return makeResponse(200, { data: 'ok' })
    }

    await _fetchOnce('/api/protected')

    const authHeader = capturedInit.headers instanceof Headers
      ? capturedInit.headers.get('authorization')
      : capturedInit.headers?.['Authorization']
    assert.ok(authHeader, 'Authorization header must be present')
    assert.match(authHeader, /^Bearer acc_old/, 'must contain Bearer + old access token')
  })

  // ═════════════════════════════════════════
  // T1.8: No Bearer token when no session
  // ═════════════════════════════════════════
  it('T1.8: does NOT inject Bearer token when not logged in', async () => {
    // No session in storage
    assert.equal(readAuthSession(), null)

    let capturedInit = null
    _fetchMock = (url, init) => {
      capturedInit = init
      return makeResponse(200, { data: 'public' })
    }

    await _fetchOnce('/api/public')

    const authHeader = capturedInit.headers instanceof Headers
      ? capturedInit.headers.get('authorization')
      : capturedInit.headers?.['Authorization']
    assert.equal(authHeader, null, 'no Authorization header when logged out')
  })
})
