import { requestJson } from './api'

const PNL_COLOR_POSITIVE = 'text-rose-400'
const PNL_COLOR_NEGATIVE = 'text-emerald-400'
const PNL_COLOR_NEUTRAL = 'text-white/42'

export function getCalendarPnlColor(value) {
  if (typeof value !== 'number' || Number.isNaN(value) || value === 0) return PNL_COLOR_NEUTRAL
  return value > 0 ? PNL_COLOR_POSITIVE : PNL_COLOR_NEGATIVE
}

export function formatCalendarPnlAmount(value, exchange = 'ASHARE') {
  if (typeof value !== 'number' || Number.isNaN(value)) return '--'
  const symbol = exchange === 'HKEX' ? 'HK$' : '¥'
  const sign = value > 0 ? '+' : value < 0 ? '-' : ''
  const abs = Math.abs(value)
  return `${sign}${symbol}${abs.toLocaleString('zh-CN', { maximumFractionDigits: 2, minimumFractionDigits: 2 })}`
}

export function formatCalendarPnlRate(value) {
  if (typeof value !== 'number' || Number.isNaN(value)) return '--'
  const sign = value > 0 ? '+' : value < 0 ? '-' : ''
  return `${sign}${Math.abs(value * 100).toFixed(2)}%`
}

export function getCalendarMonthFromDate(date = new Date()) {
  return {
    year: date.getFullYear(),
    month: date.getMonth() + 1,
  }
}

export function formatCalendarDate(date = new Date()) {
  const year = date.getFullYear()
  const month = String(date.getMonth() + 1).padStart(2, '0')
  const day = String(date.getDate()).padStart(2, '0')
  return `${year}-${month}-${day}`
}

export function resolveAvailableCalendarScopes(summary) {
  const blocks = Array.isArray(summary?.amounts_by_market) ? summary.amounts_by_market : []
  const scopes = blocks
    .map((block) => block?.scope)
    .filter((scope) => scope === 'ASHARE' || scope === 'HKEX')

  if (scopes.length > 0) return Array.from(new Set(scopes))
  if (summary?.scope === 'HKEX') return ['HKEX']
  if (summary?.scope === 'ASHARE') return ['ASHARE']
  if (summary?.amounts?.currency_code === 'HKD') return ['HKEX']
  return ['ASHARE']
}

export function resolveDefaultCalendarScope(pageScope, summary) {
  if (pageScope === 'ASHARE' || pageScope === 'HKEX') return pageScope
  const available = resolveAvailableCalendarScopes(summary)
  if (available.includes('ASHARE')) return 'ASHARE'
  if (available.includes('HKEX')) return 'HKEX'
  return 'ASHARE'
}

export async function fetchPortfolioPnlCalendar(query = {}) {
  const qs = new URLSearchParams()
  if (query.scope) qs.set('scope', query.scope)
  if (query.year) qs.set('year', String(query.year))
  if (query.month) qs.set('month', String(query.month))

  const path = `/api/portfolio/pnl-calendar${qs.toString() ? '?' + qs.toString() : ''}`
  return await requestJson(path, undefined, '加载盈亏日历失败')
}
