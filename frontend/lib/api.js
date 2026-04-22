import { getAccessToken, getRefreshToken, readAuthSession, writeAuthSession, clearAuthSession } from './auth-storage.js'

// ── Network Error Detection ──
// Distinguishes "server unreachable / timeout / CORS" from real auth/business errors.
// This is critical: after deployment the backend may be temporarily unavailable;
// we must NOT treat those transient failures as "session expired".

export function isNetworkError(error) {
  if (!error) return false
  // status === 0 means request never completed (CORS, abort, network down)
  if (error.status === 0 || error.status === undefined) return true
  // Standard HTTP status codes for server-side / proxy issues
  const s = Number(error.status)
  if (s >= 500 && s < 600) return true   // 5xx = server error (including 502 Bad Gateway)
  if (s === 502 || s === 503 || s === 504) return true // gateway/proxy errors during deployment
  return false
}

const MAX_RETRIES = 2
const RETRY_DELAY_MS = 1500

/** Guard: prevent concurrent refresh calls when multiple requests hit 401 simultaneously. */
let _refreshPromise = null

/**
 * Core fetch wrapper with:
 *  - Auto Bearer token injection
 *  - Auto-retry on network errors (max MAX_RETRIES times)
 *  - Auto-refresh on 401 (silent token rotation)
 *  - Structured error objects
 */
export async function requestJson(input, init = {}, fallbackMessage = '请求失败') {
  let lastError
  let attempt = 0

  while (attempt <= MAX_RETRIES) {
    try {
      const result = await _fetchOnce(input, init, fallbackMessage)
      return result
    } catch (err) {
      lastError = err

      // Only retry on network/transient errors — NOT on 4xx business errors
      if (!isNetworkError(err) || attempt >= MAX_RETRIES) {
        throw err
      }

      attempt++
      if (attempt <= MAX_RETRIES) {
        await new Promise((resolve) => setTimeout(resolve, RETRY_DELAY_MS * attempt))
      }
    }
  }

  throw lastError
}

/**
 * Attempt a silent token refresh.
 * Returns 'ok' | 'auth_failed' | 'network_failed':
 *   - 'ok':            token refreshed successfully
 *   - 'auth_failed':   server rejected the credentials (clear session)
 *   - 'network_failed': request failed (502, timeout, etc.) — preserve session
 * Uses a singleton promise to avoid thundering-herd when multiple API calls
 * all receive 401 at the same time.
 */
async function tryRefreshToken() {
  // If a refresh is already in flight, reuse that promise
  if (_refreshPromise) return _refreshPromise

  _refreshPromise = (async () => {
    const refreshToken = getRefreshToken()
    if (!refreshToken) {
      clearAuthSession()
      return 'auth_failed'
    }

    try {
      const res = await fetch('/api/auth/refresh', {
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

      // Merge updated tokens into existing session (preserve user info)
      const existing = readAuthSession() || {}
      const next = {
        ...existing,
        tokens: data.tokens,
        user: data.user || existing.user,
      }
      writeAuthSession(next)
      return 'ok'
    } catch {
      // Network error during refresh — don't clear session, caller decides
      return 'network_failed'
    } finally {
      _refreshPromise = null
    }
  })()

  return _refreshPromise
}

/** Single fetch attempt (no retry logic). Handles 401 → auto-refresh → retry. */

/** Safely read a fetch Response body as JSON.
 *  Handles empty bodies (204), non-JSON content, and parse errors gracefully. */
async function readApiResponse(response) {
  const text = await response.text()
  if (!text) return null
  try {
    return JSON.parse(text)
  } catch {
    // Non-JSON response (e.g., HTML error page) — return raw text wrapped
    return { detail: text }
  }
}

async function _fetchOnce(input, init, fallbackMessage) {
  const headers = new Headers(init?.headers || {})
  if (!headers.has('accept')) {
    headers.set('Accept', 'application/json')
  }

  const accessToken = getAccessToken()
  if (accessToken && !headers.has('authorization')) {
    headers.set('Authorization', `Bearer ${accessToken}`)
  }

  const response = await fetch(input, {
    ...init,
    headers,
  })
  const data = await readApiResponse(response)

  if (response.ok) {
    return data
  }

  // On 401 Unauthorized → attempt silent token refresh, then retry once
  if (response.status === 401) {
    const result = await tryRefreshToken()
    if (result === 'ok') {
      // Retry original request with fresh token
      const retryHeaders = new Headers(init?.headers || {})
      if (!retryHeaders.has('accept')) retryHeaders.set('Accept', 'application/json')
      const newToken = getAccessToken()
      if (newToken) retryHeaders.set('Authorization', `Bearer ${newToken}`)

      const retryResponse = await fetch(input, { ...init, headers: retryHeaders })
      const retryData = await readApiResponse(retryResponse)

      if (retryResponse.ok) return retryData
      if (retryResponse.status === 401) {
        // Still unauthorized after refresh — session truly expired
        clearAuthSession()
        throw buildApiError(retryResponse, retryData, fallbackMessage)
      }
      throw buildApiError(retryResponse, retryData, fallbackMessage)
    }

    // Network failure (502 / timeout / unreachable during deployment)
    if (result === 'network_failed') {
      // Preserve cached session — UI stays logged-in, will retry later
      throw { status: 0, message: '网络暂时不可用，请稍后重试', transient: true }
    }

    // auth_failed — server explicitly rejected credentials
    clearAuthSession()
  }

  throw buildApiError(response, data, fallbackMessage)
}

function buildApiError(response, responseData, fallbackText) {
  const error = new Error(extractApiErrorMessage(responseData, fallbackText))
  error.status = response?.status || 0
  error.code =
    responseData &&
    typeof responseData === 'object' &&
    !Array.isArray(responseData) &&
    typeof responseData.code === 'string'
      ? responseData.code
      : ''
  error.responseData = responseData
  return error
}

export function extractApiErrorMessage(responseData, fallbackText = '请求失败') {
  if (responseData && typeof responseData === 'object' && !Array.isArray(responseData) && 'detail' in responseData) {
    return formatApiDetail(responseData.detail) || fallbackText;
  }

  return formatApiDetail(responseData) || fallbackText;
}

function formatApiDetail(detail) {
  if (!detail) return '';
  if (typeof detail === 'string') return detail;

  if (Array.isArray(detail)) {
    return detail.map((item) => formatApiValidationItem(item)).filter(Boolean).join('；');
  }

  if (typeof detail === 'object') {
    if (typeof detail.message === 'string') return detail.message;
    if (typeof detail.detail === 'string') return detail.detail;
  }

  return String(detail);
}

function formatApiValidationItem(item) {
  if (!item || typeof item !== 'object') {
    return typeof item === 'string' ? item : String(item || '');
  }

  const fieldPath = formatErrorFieldPath(item.loc);

  if (item.type === 'greater_than_equal' && item.ctx?.ge !== undefined) {
    return `${fieldPath || '该字段'}不能小于 ${item.ctx.ge}。`;
  }

  if (item.type === 'less_than_equal' && item.ctx?.le !== undefined) {
    return `${fieldPath || '该字段'}不能大于 ${item.ctx.le}。`;
  }

  if (item.type === 'greater_than' && item.ctx?.gt !== undefined) {
    return `${fieldPath || '该字段'}必须大于 ${item.ctx.gt}。`;
  }

  if (item.type === 'less_than' && item.ctx?.lt !== undefined) {
    return `${fieldPath || '该字段'}必须小于 ${item.ctx.lt}。`;
  }

  if (item.msg) {
    return fieldPath ? `${fieldPath}：${item.msg}` : item.msg;
  }

  return fieldPath || '请求参数校验失败';
}

function formatErrorFieldPath(loc) {
  if (!Array.isArray(loc)) return '';

  const labels = {
    ticker: '股票代码',
    capital: '初始资金',
    fee_pct: '手续费率',
    strategy_params: '策略参数',
  };

  return loc
    .filter((segment) => segment !== 'body')
    .map((segment) => labels[segment] || String(segment))
    .join(' / ');
}
