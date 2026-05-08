export const QUADRANT_SEARCH_MIN_QUERY_LEN = 2
export const QUADRANT_SEARCH_MAX_RESULTS = 8

const EMPTY_QUADRANT_SEARCH_ENTRY = Object.freeze({ query: '', selectedCode: '' })

export function normalizeQuadrantMarket(market) {
  return String(market || '').toUpperCase() === 'HKEX' ? 'HKEX' : 'ASHARE'
}

export function createQuadrantSearchState() {
  return {
    ASHARE: { ...EMPTY_QUADRANT_SEARCH_ENTRY },
    HKEX: { ...EMPTY_QUADRANT_SEARCH_ENTRY },
  }
}

export function getQuadrantSearchEntry(state, market) {
  const key = normalizeQuadrantMarket(market)
  return state?.[key] || EMPTY_QUADRANT_SEARCH_ENTRY
}

export function updateQuadrantSearchEntry(state, market, patch = {}) {
  const key = normalizeQuadrantMarket(market)
  const current = getQuadrantSearchEntry(state, key)
  return {
    ...(state || createQuadrantSearchState()),
    [key]: {
      ...current,
      ...patch,
    },
  }
}

export function normalizeQuadrantSearchQuery(query) {
  return normalizeQuadrantSearchName(String(query || '').trim())
}

export function normalizeQuadrantStockCode(code, market) {
  const digits = String(code || '').replace(/\D/g, '')
  if (!digits) return ''
  const normalizedMarket = normalizeQuadrantMarket(market)
  if (normalizedMarket === 'HKEX') return digits.padStart(5, '0').slice(-5)
  return digits.padStart(6, '0').slice(-6)
}

export function buildQuadrantDetailSymbol(code, market) {
  const normalizedMarket = normalizeQuadrantMarket(market)
  const normalizedCode = normalizeQuadrantStockCode(code, normalizedMarket)
  if (!normalizedCode) return ''
  if (normalizedMarket === 'HKEX') return `${normalizedCode}.HK`
  return normalizedCode.startsWith('6') || normalizedCode.startsWith('9')
    ? `${normalizedCode}.SH`
    : `${normalizedCode}.SZ`
}

export function findQuadrantStockByCode(stocks, code, market) {
  const normalizedCode = normalizeQuadrantStockCode(code, market)
  if (!normalizedCode) return null
  const normalizedMarket = normalizeQuadrantMarket(market)
  return (stocks || []).find((stock) => normalizeQuadrantStockCode(stock?.c, normalizedMarket) === normalizedCode) || null
}

export function searchQuadrantStocks(stocks, query, market, limit = QUADRANT_SEARCH_MAX_RESULTS) {
  const normalizedQuery = normalizeQuadrantSearchQuery(query)
  if (normalizedQuery.length < QUADRANT_SEARCH_MIN_QUERY_LEN || limit <= 0) return []

  const normalizedMarket = normalizeQuadrantMarket(market)
  const queryLower = normalizedQuery.toLowerCase()
  const queryDigits = normalizedQuery.replace(/\D/g, '')
  const normalizedQueryCode = queryDigits ? normalizeQuadrantStockCode(queryDigits, normalizedMarket) : ''
  const compactQueryCode = queryDigits ? trimLeadingZeros(queryDigits) : ''

  const matches = []
  for (const stock of stocks || []) {
    const rank = getQuadrantMatchRank(stock, queryLower, normalizedQueryCode, compactQueryCode, normalizedMarket)
    if (rank === null) continue
    matches.push(stockWithRank(stock, rank, normalizedMarket))
  }

  matches.sort(compareQuadrantSearchResults)
  return matches.slice(0, limit).map(({ _matchRank, _normalizedCode, _nameLength, ...stock }) => stock)
}

function getQuadrantMatchRank(stock, queryLower, normalizedQueryCode, compactQueryCode, market) {
  const name = String(stock?.n || '').trim()
  const nameLower = normalizeQuadrantSearchName(name).toLowerCase()
  const normalizedCode = normalizeQuadrantStockCode(stock?.c, market)
  const compactCode = trimLeadingZeros(normalizedCode)

  if (normalizedQueryCode || compactQueryCode) {
    if (normalizedCode === normalizedQueryCode || compactCode === compactQueryCode) return 0
  }
  if (nameLower === queryLower) return 1
  if (normalizedQueryCode || compactQueryCode) {
    if (
      (normalizedQueryCode && normalizedCode.startsWith(normalizedQueryCode)) ||
      (compactQueryCode && compactCode.startsWith(compactQueryCode))
    ) {
      return 2
    }
  }
  if (nameLower.startsWith(queryLower)) return 3
  if (nameLower.includes(queryLower)) return 4
  if (normalizedQueryCode || compactQueryCode) {
    if (
      (normalizedQueryCode && normalizedCode.includes(normalizedQueryCode)) ||
      (compactQueryCode && compactCode.includes(compactQueryCode))
    ) {
      return 5
    }
  }
  return null
}

function stockWithRank(stock, rank, market) {
  return {
    ...stock,
    _matchRank: rank,
    _normalizedCode: normalizeQuadrantStockCode(stock?.c, market),
    _nameLength: String(stock?.n || '').trim().length,
  }
}

function compareQuadrantSearchResults(a, b) {
  if (a._matchRank !== b._matchRank) return a._matchRank - b._matchRank
  if (a._nameLength !== b._nameLength) return a._nameLength - b._nameLength
  if (a._normalizedCode.length !== b._normalizedCode.length) return a._normalizedCode.length - b._normalizedCode.length
  if (a.o !== b.o) return Number(b.o || 0) - Number(a.o || 0)
  return a._normalizedCode.localeCompare(b._normalizedCode, 'zh-CN')
}

function trimLeadingZeros(code) {
  const text = String(code || '')
  const trimmed = text.replace(/^0+/, '')
  return trimmed || '0'
}

function normalizeQuadrantSearchName(text) {
  let normalized = String(text || '').trim()
  if (!normalized) return ''
  const chineseGap = /([\u3400-\u9fff\uf900-\ufaff\uff21-\uff3a\uff41-\uff5a\uff10-\uff19])\s+([\u3400-\u9fff\uf900-\ufaff\uff21-\uff3a\uff41-\uff5a\uff10-\uff19])/g
  while (chineseGap.exec(normalized)) {
    normalized = normalized.replace(chineseGap, '$1$2')
    chineseGap.lastIndex = 0
  }
  return normalized
}
