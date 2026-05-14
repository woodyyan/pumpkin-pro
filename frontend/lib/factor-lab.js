export const DEFAULT_FACTOR_SORT = { sortBy: 'code', sortOrder: 'asc' }

export function normalizeRangeInput(value) {
  if (value === null || value === undefined) return null
  const text = String(value).trim()
  if (!text) return null
  const num = Number(text)
  if (!Number.isFinite(num)) return null
  return num
}

export function buildFactorScreenerPayload({ filters = {}, sortBy = 'code', sortOrder = 'asc', page = 1, pageSize = 50, snapshotDate = '' } = {}) {
  const normalizedFilters = {}
  for (const [key, range] of Object.entries(filters || {})) {
    const min = normalizeRangeInput(range?.min)
    const max = normalizeRangeInput(range?.max)
    if (min === null && max === null) continue
    normalizedFilters[key] = {}
    if (min !== null) normalizedFilters[key].min = min
    if (max !== null) normalizedFilters[key].max = max
  }
  return {
    snapshot_date: snapshotDate || '',
    filters: normalizedFilters,
    sort_by: sortBy || 'code',
    sort_order: sortOrder === 'desc' ? 'desc' : 'asc',
    page: Math.max(1, Number(page) || 1),
    page_size: Math.min(200, Math.max(1, Number(pageSize) || 50)),
  }
}

export function updateFactorFilter(filters, key, bound, value) {
  const next = { ...(filters || {}) }
  const current = { ...(next[key] || {}) }
  current[bound] = value
  const min = normalizeRangeInput(current.min)
  const max = normalizeRangeInput(current.max)
  if (min === null && max === null) {
    delete next[key]
  } else {
    next[key] = current
  }
  return next
}

export function removeFactorFilter(filters, key) {
  const next = { ...(filters || {}) }
  delete next[key]
  return next
}

export function flattenMetricDefinitions(groups = []) {
  const map = {}
  for (const group of groups || []) {
    for (const item of group.items || []) {
      map[item.key] = { ...item, group: group.key, groupLabel: group.label }
    }
  }
  return map
}

export function getDynamicMetricColumns(filters = {}, metricMap = {}) {
  const keys = Object.keys(filters || {})
  return keys.filter((key) => metricMap[key]).slice(0, 6)
}

export function buildSelectedFilterChips(filters = {}, metricMap = {}) {
  return Object.entries(filters || {}).map(([key, range]) => {
    const metric = metricMap[key] || { label: key, unit: '' }
    const parts = []
    if (normalizeRangeInput(range?.min) !== null) parts.push(`≥ ${range.min}${metric.unit || ''}`)
    if (normalizeRangeInput(range?.max) !== null) parts.push(`≤ ${range.max}${metric.unit || ''}`)
    return { key, label: metric.label, text: `${metric.label} ${parts.join(' ')}`.trim() }
  })
}

export function formatFactorValue(value, format = 'number') {
  if (value === null || value === undefined || value === '') return '--'
  const num = Number(value)
  if (!Number.isFinite(num)) return '--'
  switch (format) {
    case 'price':
      return num.toFixed(2)
    case 'percent':
      return `${num >= 0 ? '+' : ''}${num.toFixed(2)}%`
    case 'percentFromRatio':
      return `${(num * 100).toFixed(2)}%`
    case 'bigNumber': {
      const abs = Math.abs(num)
      if (abs >= 1e8) return `${(num / 1e8).toFixed(2)} 亿`
      if (abs >= 1e4) return `${(num / 1e4).toFixed(2)} 万`
      return num.toFixed(2)
    }
    case 'integer':
      return num.toLocaleString('zh-CN', { maximumFractionDigits: 0 })
    case 'number':
    default:
      return num.toFixed(2)
  }
}

export function getBoardLabel(board) {
  switch (board) {
    case 'MAIN': return '主板'
    case 'CHINEXT': return '创业板'
    case 'STAR': return '科创板'
    case 'BJ': return '北交所'
    default: return board || '--'
  }
}

export function getPageNumbers(current, total) {
  if (total <= 7) return Array.from({ length: total }, (_, i) => i + 1)
  const pages = [1]
  if (current > 3) pages.push('...')
  for (let i = Math.max(2, current - 1); i <= Math.min(total - 1, current + 1); i += 1) pages.push(i)
  if (current < total - 2) pages.push('...')
  pages.push(total)
  return pages
}

export function codeToSymbol(code) {
  const c = String(code || '').padStart(6, '0')
  return c.startsWith('6') || c.startsWith('9') ? `${c}.SH` : `${c}.SZ`
}
