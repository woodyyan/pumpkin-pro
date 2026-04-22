import { requestJson } from './api.js'

export const ATTRIBUTION_RANGE_OPTIONS = [
  { value: '7D', label: '7天' },
  { value: '30D', label: '30天' },
  { value: '90D', label: '90天' },
  { value: 'ALL', label: '全部' },
]

const DEFAULT_DETAIL_LIMIT = 3

export function createAttributionDetailSectionsState(overrides = {}) {
  return {
    marketExpanded: true,
    sectorExpanded: true,
    ...overrides,
  }
}

export function buildAttributionDetailRequestKeys({ detailOpen = false, marketExpanded = true, sectorExpanded = true } = {}) {
  if (!detailOpen) return []

  const keys = ['stocks', 'trading']
  if (marketExpanded) keys.push('market')
  if (sectorExpanded) keys.push('sectors')
  return keys
}

export function resolveAttributionActiveScope(availableScopes = [], currentScope, fallbackScope = null) {
  const scopes = Array.isArray(availableScopes)
    ? availableScopes.map((item) => item?.scope).filter(Boolean)
    : []

  if (currentScope && scopes.includes(currentScope)) return currentScope
  if (fallbackScope && scopes.includes(fallbackScope)) return fallbackScope
  return scopes[0] || fallbackScope || null
}

export function buildPortfolioAttributionQuery(query = {}) {
  const qs = new URLSearchParams()
  if (query.scope) qs.set('scope', query.scope)
  if (query.range) qs.set('range', query.range)
  if (query.start_date) qs.set('start_date', query.start_date)
  if (query.end_date) qs.set('end_date', query.end_date)
  if (query.limit) qs.set('limit', String(query.limit))
  if (query.sort_by) qs.set('sort_by', query.sort_by)
  if (query.refresh) qs.set('refresh', 'true')
  if (query.include_unclassified) qs.set('include_unclassified', 'true')
  if (query.timeline_limit) qs.set('timeline_limit', String(query.timeline_limit))
  return qs.toString()
}

export function buildPortfolioAttributionPath(endpoint, query = {}) {
  const qs = buildPortfolioAttributionQuery(query)
  return `/api/portfolio/attribution/${endpoint}${qs ? `?${qs}` : ''}`
}

async function requestPortfolioAttribution(endpoint, query, fallbackMessage) {
  return await requestJson(buildPortfolioAttributionPath(endpoint, query), undefined, fallbackMessage)
}

export async function fetchPortfolioAttributionSummary(query = {}) {
  return await requestPortfolioAttribution('summary', query, '加载绩效归因总览失败')
}

export async function fetchPortfolioAttributionStocks(query = {}) {
  return await requestPortfolioAttribution('stocks', query, '加载个股归因失败')
}

export async function fetchPortfolioAttributionSectors(query = {}) {
  return await requestPortfolioAttribution('sectors', query, '加载行业归因失败')
}

export async function fetchPortfolioAttributionTrading(query = {}) {
  return await requestPortfolioAttribution('trading', query, '加载交易归因失败')
}

export async function fetchPortfolioAttributionMarket(query = {}) {
  return await requestPortfolioAttribution('market', query, '加载市场归因失败')
}

export function formatAttributionScopeLabel(scope) {
  switch (scope) {
    case 'ASHARE':
      return 'A股'
    case 'HKEX':
      return '港股'
    default:
      return '全部'
  }
}

export function formatAttributionMoney(value, currencySymbol = '', options = {}) {
  const { signed = true, compact = false, fallback = '--' } = options
  if (typeof value !== 'number' || Number.isNaN(value)) return fallback
  const abs = Math.abs(value)
  const sign = signed ? (value > 0 ? '+' : value < 0 ? '-' : '') : (value < 0 ? '-' : '')

  if (compact) {
    if (abs >= 1e8) return `${sign}${currencySymbol}${(abs / 1e8).toFixed(abs >= 1e9 ? 1 : 2)}亿`
    if (abs >= 1e4) return `${sign}${currencySymbol}${(abs / 1e4).toFixed(abs >= 1e6 ? 1 : 2)}万`
  }

  return `${sign}${currencySymbol}${abs.toLocaleString('zh-CN', { maximumFractionDigits: 2, minimumFractionDigits: 2 })}`
}

export function formatAttributionPercent(value, digits = 1, options = {}) {
  const { signed = true, fallback = '--' } = options
  if (typeof value !== 'number' || Number.isNaN(value)) return fallback
  const pct = Math.abs(value) * 100
  const sign = signed ? (value > 0 ? '+' : value < 0 ? '-' : '') : ''
  return `${sign}${pct.toFixed(digits)}%`
}

export function attributionToneClass(value, fallback = 'text-white/82') {
  if (typeof value !== 'number' || Number.isNaN(value)) return fallback
  if (value > 0) return 'text-rose-400'
  if (value < 0) return 'text-emerald-400'
  return fallback
}

export function attributionSeverityClass(severity) {
  switch (severity) {
    case 'positive':
      return 'border-rose-500/20 bg-rose-500/[0.06] text-rose-200'
    case 'warning':
      return 'border-amber-500/20 bg-amber-500/[0.08] text-amber-100'
    default:
      return 'border-white/10 bg-white/[0.03] text-white/78'
  }
}

export function groupByScope(items = []) {
  const result = new Map()
  for (const item of Array.isArray(items) ? items : []) {
    const key = item?.scope || 'ALL'
    result.set(key, item)
  }
  return result
}

function isNumber(value) {
  return typeof value === 'number' && Number.isFinite(value)
}

function sortByAbsAmountDesc(a, b) {
  return Math.abs(b?.amount || 0) - Math.abs(a?.amount || 0)
}

function sortByPositiveDesc(a, b) {
  return (b?.amount || 0) - (a?.amount || 0)
}

function sortByNegativeAsc(a, b) {
  return (a?.amount || 0) - (b?.amount || 0)
}

function isTotalWaterfallItem(item) {
  return item?.type === 'total' || item?.key === 'total' || item?.key === 'total_pnl'
}

function mapScopeMeta(group) {
  return {
    scope: group?.scope || 'ALL',
    label: group?.scope_label || formatAttributionScopeLabel(group?.scope),
  }
}

export function pickAttributionScopeItem(items = [], preferredScope) {
  const groups = Array.isArray(items) ? items.filter(Boolean) : []
  if (!groups.length) return null
  if (preferredScope) {
    const matched = groups.find((item) => item?.scope === preferredScope)
    if (matched) return matched
  }
  return groups[0]
}

export function buildAttributionHero(summary, preferredScope) {
  const groups = Array.isArray(summary?.waterfall_groups) ? summary.waterfall_groups : []
  const activeGroup = pickAttributionScopeItem(groups, preferredScope)
  const scopes = groups.map(mapScopeMeta)
  const items = Array.isArray(activeGroup?.items) ? activeGroup.items : []
  const contributionItems = items.filter((item) => !isTotalWaterfallItem(item))
  const totalItem = items.find(isTotalWaterfallItem) || null
  const totalAmount = isNumber(totalItem?.amount)
    ? totalItem.amount
    : contributionItems.reduce((sum, item) => sum + (isNumber(item?.amount) ? item.amount : 0), 0)
  const primaryDriver = [...contributionItems].filter((item) => (item?.amount || 0) > 0).sort(sortByPositiveDesc)[0] || null
  const biggestDrag = [...contributionItems].filter((item) => (item?.amount || 0) < 0).sort(sortByNegativeAsc)[0] || null

  return {
    availableScopes: scopes,
    activeScope: activeGroup?.scope || preferredScope || null,
    activeGroup,
    totalAmount,
    currencySymbol: activeGroup?.currency_symbol || totalItem?.currency_symbol || '',
    primaryDriver,
    biggestDrag,
    headline: summary?.headline || '',
  }
}

export function buildAttributionWaterfallSeries(group) {
  const items = Array.isArray(group?.items) ? group.items : []
  let running = 0

  return items.map((item) => {
    const amount = isNumber(item?.amount) ? item.amount : 0
    if (isTotalWaterfallItem(item)) {
      return {
        ...item,
        amount,
        start: 0,
        end: amount,
        isTotal: true,
      }
    }

    const start = running
    const end = running + amount
    running = end
    return {
      ...item,
      amount,
      start,
      end,
      isTotal: false,
    }
  })
}

export function buildAttributionHeroBadges(summary, preferredScope) {
  const hero = buildAttributionHero(summary, preferredScope)
  const badges = [
    {
      key: 'total',
      label: '总收益',
      toneValue: hero.totalAmount,
      value: formatAttributionMoney(hero.totalAmount, hero.currencySymbol, { compact: true }),
    },
    {
      key: 'driver',
      label: '主驱动',
      toneValue: hero.primaryDriver?.amount || 0,
      value: hero.primaryDriver?.label || '暂无明显主驱动',
      subValue: hero.primaryDriver ? formatAttributionMoney(hero.primaryDriver.amount, hero.currencySymbol, { compact: true }) : '',
    },
    {
      key: 'drag',
      label: '最大拖累',
      toneValue: hero.biggestDrag?.amount || 0,
      value: hero.biggestDrag?.label || '暂无明显拖累',
      subValue: hero.biggestDrag ? formatAttributionMoney(hero.biggestDrag.amount, hero.currencySymbol, { compact: true }) : '',
    },
  ]

  return {
    ...hero,
    badges,
  }
}

export function pickAttributionStockHighlights(payload, preferredScope, limit = DEFAULT_DETAIL_LIMIT) {
  const positiveGroup = pickAttributionScopeItem(payload?.positive_groups, preferredScope)
  const negativeGroup = pickAttributionScopeItem(payload?.negative_groups, preferredScope)

  return {
    scope: positiveGroup?.scope || negativeGroup?.scope || preferredScope || 'ALL',
    scopeLabel: positiveGroup?.scope_label || negativeGroup?.scope_label || formatAttributionScopeLabel(preferredScope),
    currencySymbol: positiveGroup?.currency_symbol || negativeGroup?.currency_symbol || '',
    positive: Array.isArray(positiveGroup?.items) ? positiveGroup.items.slice(0, limit) : [],
    negative: Array.isArray(negativeGroup?.items) ? negativeGroup.items.slice(0, limit) : [],
  }
}

export function pickAttributionTradingHighlights(payload, preferredScope, limit = DEFAULT_DETAIL_LIMIT) {
  const group = pickAttributionScopeItem(payload?.groups, preferredScope)
  const timeline = Array.isArray(group?.timeline) ? group.timeline : []
  const sorted = [...timeline].filter(Boolean).sort((a, b) => sortByAbsAmountDesc({ amount: a?.timing_effect_amount }, { amount: b?.timing_effect_amount }))

  return {
    group,
    positive: sorted.filter((item) => (item?.timing_effect_amount || 0) > 0).slice(0, limit),
    negative: sorted.filter((item) => (item?.timing_effect_amount || 0) < 0).slice(0, limit),
  }
}

export function pickAttributionMarketSnapshot(payload, preferredScope) {
  return pickAttributionScopeItem(payload?.groups, preferredScope)
}

export function pickAttributionSectorHighlights(payload, preferredScope, limit = DEFAULT_DETAIL_LIMIT) {
  const group = pickAttributionScopeItem(payload?.groups, preferredScope)
  const items = Array.isArray(group?.items) ? group.items : []

  return {
    group,
    positive: [...items].filter((item) => (item?.total_pnl_amount || 0) > 0).sort((a, b) => (b?.total_pnl_amount || 0) - (a?.total_pnl_amount || 0)).slice(0, limit),
    negative: [...items].filter((item) => (item?.total_pnl_amount || 0) < 0).sort((a, b) => (a?.total_pnl_amount || 0) - (b?.total_pnl_amount || 0)).slice(0, limit),
  }
}
