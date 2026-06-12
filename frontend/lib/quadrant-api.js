function normalizeQuadrantApiExchange(exchange) {
  return String(exchange || '').trim().toUpperCase() === 'HKEX' ? 'HKEX' : 'ASHARE'
}

function normalizeWatchlistSymbols(watchlistSymbols) {
  if (!Array.isArray(watchlistSymbols)) return []

  const deduped = new Set()
  for (const symbol of watchlistSymbols) {
    const normalized = String(symbol || '').trim()
    if (normalized) deduped.add(normalized)
  }
  return [...deduped]
}

export function buildQuadrantUrl({ exchange = 'ASHARE', watchlistSymbols = [] } = {}) {
  const params = new URLSearchParams()
  if (normalizeQuadrantApiExchange(exchange) === 'HKEX') {
    params.set('exchange', 'HKEX')
  }

  const normalizedWatchlist = normalizeWatchlistSymbols(watchlistSymbols)
  if (normalizedWatchlist.length > 0) {
    params.set('watchlist_symbols', normalizedWatchlist.join(','))
  }

  const qs = params.toString()
  return `/api/quadrant${qs ? `?${qs}` : ''}`
}

export function buildQuadrantRankingUrl(exchange, limit = 20) {
  const params = new URLSearchParams()
  params.set('limit', String(limit > 0 ? limit : 20))
  if (normalizeQuadrantApiExchange(exchange) === 'HKEX') {
    params.set('exchange', 'HKEX')
  }
  return `/api/quadrant/ranking?${params.toString()}`
}
