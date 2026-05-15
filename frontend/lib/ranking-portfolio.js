export function formatRankingPortfolioPercent(value, digits = 2) {
  if (value === null || value === undefined || Number.isNaN(Number(value))) return '--'
  const num = Number(value)
  const sign = num > 0 ? '+' : ''
  return `${sign}${num.toFixed(digits)}%`
}

function parseRankingPortfolioNumber(value) {
  const num = Number(value)
  return Number.isFinite(num) ? num : null
}

function resolveRankingPortfolioReturnPct(item, returnKey, navKey) {
  const directValue = parseRankingPortfolioNumber(item?.[returnKey])
  if (directValue !== null) return directValue

  const nav = parseRankingPortfolioNumber(item?.[navKey])
  if (nav === null) return null
  return (nav - 1) * 100
}

export function getRankingPortfolioPerformanceClass(value) {
  if (value === null || value === undefined || Number.isNaN(Number(value))) return 'text-white/35'
  return Number(value) >= 0 ? 'text-rose-300' : 'text-emerald-300'
}

export function formatRankingPortfolioDateTime(value) {
  if (!value) return '--'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return date.toLocaleString('zh-CN', { hour12: false })
}

export function formatRankingPortfolioDate(value) {
  if (!value) return '--'
  if (typeof value === 'string') {
    if (/^\d{4}-\d{2}-\d{2}$/.test(value)) return value
    const date = new Date(value)
    if (Number.isNaN(date.getTime())) return value
    const year = date.getFullYear()
    const month = String(date.getMonth() + 1).padStart(2, '0')
    const day = String(date.getDate()).padStart(2, '0')
    return `${year}-${month}-${day}`
  }
  if (typeof value === 'object' && value.year && value.month && value.day) {
    return `${String(value.year).padStart(4, '0')}-${String(value.month).padStart(2, '0')}-${String(value.day).padStart(2, '0')}`
  }
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return '--'
  const year = date.getFullYear()
  const month = String(date.getMonth() + 1).padStart(2, '0')
  const day = String(date.getDate()).padStart(2, '0')
  return `${year}-${month}-${day}`
}

export function formatRankingPortfolioCode(code, exchange) {
  if (exchange === 'HKEX') return String(code || '').padStart(5, '0')
  return String(code || '')
}

export function buildRankingPortfolioDetailSymbol(code, exchange) {
  const digits = String(code || '').replace(/\D/g, '')
  if (!digits) return ''

  const normalizedExchange = String(exchange || '').toUpperCase()
  if (normalizedExchange === 'HKEX') {
    return `${digits.padStart(5, '0').slice(-5)}.HK`
  }

  const normalizedCode = digits.padStart(6, '0').slice(-6)
  if (normalizedExchange === 'SSE') return `${normalizedCode}.SH`
  if (normalizedExchange === 'SZSE') return `${normalizedCode}.SZ`

  return normalizedCode.startsWith('6') || normalizedCode.startsWith('9')
    ? `${normalizedCode}.SH`
    : `${normalizedCode}.SZ`
}

export function buildRankingPortfolioDetailHref(code, exchange) {
  const symbol = buildRankingPortfolioDetailSymbol(code, exchange)
  return symbol ? `/live-trading/${encodeURIComponent(symbol)}` : ''
}

export function buildRankingPortfolioChartPoints(series, width, height, padding) {
  if (!Array.isArray(series) || series.length === 0) return { portfolio: '', benchmark: '', baselineY: height / 2 }
  const values = []
  for (const item of series) {
    if (Number.isFinite(Number(item.nav))) values.push(Number(item.nav))
    if (Number.isFinite(Number(item.benchmark_nav))) values.push(Number(item.benchmark_nav))
  }
  const minValue = Math.min(...values, 1)
  const maxValue = Math.max(...values, 1)
  const range = Math.max(maxValue - minValue, 0.001)
  const innerWidth = Math.max(width - padding * 2, 1)
  const innerHeight = Math.max(height - padding * 2, 1)

  const buildPath = (key) => series.map((item, index) => {
    const x = padding + (innerWidth * index) / Math.max(series.length - 1, 1)
    const value = Number(item[key] || 0)
    const y = padding + innerHeight - ((value - minValue) / range) * innerHeight
    return `${index === 0 ? 'M' : 'L'} ${x.toFixed(2)} ${y.toFixed(2)}`
  }).join(' ')

  const baselineY = padding + innerHeight - ((1 - minValue) / range) * innerHeight
  return {
    portfolio: buildPath('nav'),
    benchmark: buildPath('benchmark_nav'),
    baselineY,
  }
}

export function normalizeRankingPortfolioSeries(series) {
  if (!Array.isArray(series) || series.length === 0) return []

  return [...series]
    .filter((item) => item?.date)
    .map((item) => ({
      ...item,
      __normalized_date: formatRankingPortfolioDate(item.date),
    }))
    .sort((left, right) => String(left.__normalized_date).localeCompare(String(right.__normalized_date)))
    .map((item) => {
      const portfolioReturnPct = resolveRankingPortfolioReturnPct(item, 'portfolio_return_pct', 'nav')
      const benchmarkReturnPct = resolveRankingPortfolioReturnPct(item, 'benchmark_return_pct', 'benchmark_nav')
      const excessReturnPct = parseRankingPortfolioNumber(item?.excess_return_pct)

      return {
        date: item.__normalized_date,
        portfolioReturnPct,
        benchmarkReturnPct,
        excessReturnPct: excessReturnPct !== null ? excessReturnPct : (portfolioReturnPct !== null && benchmarkReturnPct !== null ? portfolioReturnPct - benchmarkReturnPct : null),
      }
    })
    .filter((item) => item.portfolioReturnPct !== null || item.benchmarkReturnPct !== null)
}

export function buildRankingPortfolioChartSeriesData(series) {
  const points = normalizeRankingPortfolioSeries(series)
  if (!points.length) {
    return {
      points: [],
      portfolio: [],
      benchmark: [],
      baseline: [],
      latest: null,
    }
  }

  return {
    points,
    portfolio: points
      .filter((item) => item.portfolioReturnPct !== null)
      .map((item) => ({ time: item.date, value: item.portfolioReturnPct })),
    benchmark: points
      .filter((item) => item.benchmarkReturnPct !== null)
      .map((item) => ({ time: item.date, value: item.benchmarkReturnPct })),
    baseline: points.map((item) => ({ time: item.date, value: 0 })),
    latest: points[points.length - 1],
  }
}

export function findRankingPortfolioPointByTime(series, time) {
  const formattedTime = formatRankingPortfolioDate(time)
  if (formattedTime === '--') return null
  const points = normalizeRankingPortfolioSeries(series)
  return points.find((item) => item.date === formattedTime) || null
}
