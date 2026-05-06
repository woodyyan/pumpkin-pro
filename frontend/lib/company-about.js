import { requestJson } from './api.js'

export function buildCompanyAboutPath(symbol) {
  const normalized = String(symbol || '').trim().toUpperCase()
  if (!normalized) return ''
  return `/api/live/symbols/${encodeURIComponent(normalized)}/about`
}

export async function fetchCompanyAbout(symbol) {
  const path = buildCompanyAboutPath(symbol)
  if (!path) return null
  return await requestJson(path, undefined, '加载公司资料失败')
}

export function formatListingStatus(status) {
  const normalized = String(status || '').trim().toUpperCase()
  const labels = {
    LISTED: '已上市',
    DELISTED: '已退市',
    SUSPENDED: '暂停上市',
    UNKNOWN: '未确认',
  }
  return labels[normalized] || '未确认'
}

export function listingStatusTone(status) {
  const normalized = String(status || '').trim().toUpperCase()
  if (normalized === 'LISTED') return 'listed'
  if (normalized === 'DELISTED') return 'delisted'
  if (normalized === 'SUSPENDED') return 'suspended'
  return 'unknown'
}

export function formatAboutDate(value, precision = 'day') {
  const text = String(value || '').trim()
  if (!text) return '--'
  const p = String(precision || 'day').trim().toLowerCase()
  if (p === 'year') return text.slice(0, 4) || '--'
  if (p === 'month') return text.slice(0, 7) || '--'
  return text.slice(0, 10) || '--'
}

export function extractDisplayDomain(website) {
  const text = String(website || '').trim()
  if (!text) return ''
  try {
    const url = new URL(text.startsWith('http://') || text.startsWith('https://') ? text : `https://${text}`)
    return url.hostname.replace(/^www\./, '')
  } catch {
    return text.replace(/^https?:\/\//, '').replace(/^www\./, '').split('/')[0]
  }
}

export function isSafeWebsiteUrl(website) {
  const text = String(website || '').trim()
  if (!text) return false
  try {
    const url = new URL(text.startsWith('http://') || text.startsWith('https://') ? text : `https://${text}`)
    return url.protocol === 'http:' || url.protocol === 'https:'
  } catch {
    return false
  }
}

export function normalizeWebsiteHref(website) {
  const text = String(website || '').trim()
  if (!isSafeWebsiteUrl(text)) return ''
  return text.startsWith('http://') || text.startsWith('https://') ? text : `https://${text}`
}
