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

function resolveRankingPortfolioReturnPct(item) {
  const directValue = parseRankingPortfolioNumber(item?.portfolio_return_pct)
  if (directValue !== null) return directValue

  const nav = parseRankingPortfolioNumber(item?.nav)
  if (nav === null) return null
  return (nav - 1) * 100
}

function resolveRankingPortfolioDrawdownPct(item, returnPct, peakReturnPct) {
  const directValue = parseRankingPortfolioNumber(item?.drawdown_pct)
  if (directValue !== null) return directValue
  if (returnPct === null || peakReturnPct === null) return null
  return returnPct - peakReturnPct
}

export function getRankingPortfolioPerformanceClass(value) {
  if (value === null || value === undefined || Number.isNaN(Number(value))) return 'text-foreground-dim'
  return Number(value) >= 0 ? 'text-negative' : 'text-positive'
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

export function formatRankingPortfolioWeight(value, digits = 0) {
  if (value === null || value === undefined || Number.isNaN(Number(value))) return '--'
  return `${(Number(value) * 100).toFixed(digits)}%`
}

export function formatRankingPortfolioWeightChange(fromWeight, toWeight, digits = 0) {
  return `${formatRankingPortfolioWeight(fromWeight, digits)} -> ${formatRankingPortfolioWeight(toWeight, digits)}`
}

export function formatRankingPortfolioReferencePrice(value, exchange) {
  if (value === null || value === undefined || Number.isNaN(Number(value))) return '--'
  const prefix = String(exchange || '').toUpperCase() === 'HKEX' ? 'HK$' : '¥'
  return `${prefix}${Number(value).toFixed(2)}`
}

export function getRankingPortfolioRebalanceActionLabel(action) {
  return String(action || '').toLowerCase() === 'sell' ? '卖出' : '买入'
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
  if (!Array.isArray(series) || series.length === 0) return { portfolio: '', baselineY: height / 2 }
  const values = []
  for (const item of series) {
    if (Number.isFinite(Number(item.nav))) values.push(Number(item.nav))
  }
  const minValue = Math.min(...values, 1)
  const maxValue = Math.max(...values, 1)
  const range = Math.max(maxValue - minValue, 0.001)
  const innerWidth = Math.max(width - padding * 2, 1)
  const innerHeight = Math.max(height - padding * 2, 1)

  const portfolio = series.map((item, index) => {
    const x = padding + (innerWidth * index) / Math.max(series.length - 1, 1)
    const value = Number(item.nav || 0)
    const y = padding + innerHeight - ((value - minValue) / range) * innerHeight
    return `${index === 0 ? 'M' : 'L'} ${x.toFixed(2)} ${y.toFixed(2)}`
  }).join(' ')

  const baselineY = padding + innerHeight - ((1 - minValue) / range) * innerHeight
  return { portfolio, baselineY }
}

export function normalizeRankingPortfolioSeries(series) {
  if (!Array.isArray(series) || series.length === 0) return []

  let peakReturnPct = 0
  return [...series]
    .filter((item) => item?.date)
    .map((item) => ({
      ...item,
      __normalized_date: formatRankingPortfolioDate(item.date),
    }))
    .sort((left, right) => String(left.__normalized_date).localeCompare(String(right.__normalized_date)))
    .map((item) => {
      const portfolioReturnPct = resolveRankingPortfolioReturnPct(item)
      if (portfolioReturnPct !== null && portfolioReturnPct > peakReturnPct) {
        peakReturnPct = portfolioReturnPct
      }
      const dailyReturnPct = parseRankingPortfolioNumber(item?.daily_portfolio_return_pct)
      const drawdownPct = resolveRankingPortfolioDrawdownPct(item, portfolioReturnPct, peakReturnPct)

      const normalizedPoint = {
        date: item.__normalized_date,
        portfolioReturnPct,
        dailyPortfolioReturnPct: dailyReturnPct,
        drawdownPct,
      }

      if (item.source_trade_date) {
        normalizedPoint.sourceTradeDate = item.source_trade_date
      }

      return normalizedPoint
    })
    .filter((item) => item.portfolioReturnPct !== null)
}

export function buildRankingPortfolioChartSeriesData(series) {
  const points = normalizeRankingPortfolioSeries(series)
  if (!points.length) {
    return {
      points: [],
      portfolio: [],
      baseline: [],
      latest: null,
    }
  }

  return {
    points,
    portfolio: points.map((item) => ({ time: item.date, value: item.portfolioReturnPct })),
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
