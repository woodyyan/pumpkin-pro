const AUTH_SESSION_STORAGE_KEY = 'pumpkin_pro_auth_session'

function isBrowser() {
  return typeof window !== 'undefined' && !!window.localStorage
}

export function readAuthSession() {
  if (!isBrowser()) return null
  const text = window.localStorage.getItem(AUTH_SESSION_STORAGE_KEY)
  if (!text) return null
  try {
    const parsed = JSON.parse(text)
    if (!parsed || typeof parsed !== 'object') return null
    if (!parsed.tokens?.access_token || !parsed.tokens?.refresh_token) return null
    return parsed
  } catch {
    return null
  }
}

export function writeAuthSession(session) {
  if (!isBrowser()) return
  if (!session) {
    window.localStorage.removeItem(AUTH_SESSION_STORAGE_KEY)
    return
  }
  window.localStorage.setItem(AUTH_SESSION_STORAGE_KEY, JSON.stringify(session))
}

export function clearAuthSession() {
  if (!isBrowser()) return
  window.localStorage.removeItem(AUTH_SESSION_STORAGE_KEY)
}

export function getAccessToken() {
  return readAuthSession()?.tokens?.access_token || ''
}

export function getRefreshToken() {
  return readAuthSession()?.tokens?.refresh_token || ''
}

export function isAuthRequiredError(error) {
  if (!error) return false
  if (Number(error.status) === 401) return true

  const code = String(error.code || '').toUpperCase()
  if (code === 'AUTH_REQUIRED' || code === 'UNAUTHORIZED') return true

  const message = String(error.message || '')
  return message.includes('需要登录') || message.includes('登录后使用')
}
