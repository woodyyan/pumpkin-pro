export const FACTOR_DEFINITIONS = [
  { key: 'value', scoreKey: 'value_score', label: '价值' },
  { key: 'dividend_yield', scoreKey: 'dividend_yield_score', label: '股息率' },
  { key: 'growth', scoreKey: 'growth_score', label: '成长' },
  { key: 'quality', scoreKey: 'quality_score', label: '质量' },
  { key: 'momentum', scoreKey: 'momentum_score', label: '动量' },
  { key: 'size', scoreKey: 'size_score', label: '规模' },
  { key: 'low_volatility', scoreKey: 'low_volatility_score', label: '低波动' },
]

export const DEFAULT_FACTOR_SORT = { sortBy: 'composite_score', sortOrder: 'desc' }

export function normalizeWeightInput(value) {
  if (value === null || value === undefined) return null
  const text = String(value).trim()
  if (!text) return null
  const num = Number(text)
  if (!Number.isFinite(num) || num < 0) return null
  return num
}

export function normalizeFactorWeights(factorWeights = {}) {
  const normalized = {}
  for (const [key, value] of Object.entries(factorWeights || {})) {
    const num = normalizeWeightInput(value)
    if (num === null || num <= 0) continue
    normalized[key] = num
  }
  return normalized
}

export function sumFactorWeights(factorWeights = {}) {
  return Object.values(normalizeFactorWeights(factorWeights)).reduce((sum, value) => sum + value, 0)
}

export function validateFactorWeights(factorWeights = {}) {
  const normalized = normalizeFactorWeights(factorWeights)
  const selectedCount = Object.keys(factorWeights || {}).length
  if (selectedCount === 0) return { valid: true, sum: 0, message: '未选择因子时默认按 7 个因子等权综合排序。' }
  const sum = Object.values(normalized).reduce((total, value) => total + value, 0)
  if (Object.keys(normalized).length !== selectedCount) return { valid: false, sum, message: '已选择的因子需要填写大于 0 的权重。' }
  if (Math.abs(sum - 1) > 0.001) return { valid: false, sum, message: '因子权重合计必须等于 1。' }
  return { valid: true, sum, message: '权重合计为 1，将按自定义综合得分排序。' }
}

export function buildFactorScreenerPayload({ factorWeights = {}, sortBy = 'composite_score', sortOrder = 'desc', page = 1, pageSize = 50, snapshotDate = '' } = {}) {
  return {
    snapshot_date: snapshotDate || '',
    factor_weights: normalizeFactorWeights(factorWeights),
    sort_by: sortBy || 'composite_score',
    sort_order: sortOrder === 'asc' ? 'asc' : 'desc',
    page: Math.max(1, Number(page) || 1),
    page_size: Math.min(200, Math.max(1, Number(pageSize) || 50)),
  }
}

export function factorWeightChipText(factorWeights = {}, factorMap = {}) {
  const selectedCount = Object.keys(factorWeights || {}).length
  const entries = Object.entries(normalizeFactorWeights(factorWeights))
  if (entries.length === 0) return selectedCount === 0 ? ['默认 7 因子等权'] : ['请填写因子权重']
  return entries.map(([key, weight]) => `${factorMap[key]?.label || key} ${formatWeight(weight)}`)
}

export function formatWeight(value) {
  const num = Number(value)
  if (!Number.isFinite(num)) return '--'
  return num.toFixed(2)
}

export function formatFactorValue(value, format = 'number') {
  if (value === null || value === undefined || value === '') return '--'
  const num = Number(value)
  if (!Number.isFinite(num)) return '--'
  switch (format) {
    case 'score':
      return num.toFixed(1)
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

export function getScoreTone(value) {
  const num = Number(value)
  if (!Number.isFinite(num)) return 'muted'
  if (num >= 80) return 'strong'
  if (num >= 60) return 'good'
  if (num >= 40) return 'neutral'
  return 'weak'
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

export function flattenFactorDefinitions(factors = FACTOR_DEFINITIONS) {
  const map = {}
  for (const factor of factors || []) {
    map[factor.key] = factor
    if (factor.scoreKey) map[factor.scoreKey] = factor
  }
  return map
}
