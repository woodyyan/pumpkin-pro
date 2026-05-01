import { getAccessToken } from './auth-storage.js'

export function buildPageViewHeaders() {
  const headers = { 'Content-Type': 'application/json' }
  const accessToken = getAccessToken()
  if (accessToken) {
    headers.Authorization = `Bearer ${accessToken}`
  }
  return headers
}
