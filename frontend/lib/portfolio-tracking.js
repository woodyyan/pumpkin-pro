export const PORTFOLIO_TRACKING_MARKETS = {
  ASHARE: {
    key: 'ASHARE',
    label: 'A股',
    accentClass: 'text-negative',
    badgeClass: 'border-negative/20 bg-negative/10 text-negative',
  },
  HKEX: {
    key: 'HKEX',
    label: '港股',
    accentClass: 'text-sky-600',
    badgeClass: 'border-sky-500/20 bg-sky-500/10 text-sky-700',
  },
}

export function formatPortfolioTrackingDate(value) {
  if (!value) return '--'
  if (typeof value === 'string' && /^\d{4}-\d{2}-\d{2}$/.test(value)) return value
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return String(value)
  const year = date.getFullYear()
  const month = String(date.getMonth() + 1).padStart(2, '0')
  const day = String(date.getDate()).padStart(2, '0')
  return `${year}-${month}-${day}`
}

export function formatPortfolioTrackingPercent(value, digits = 2) {
  if (value === null || value === undefined || Number.isNaN(Number(value))) return '--'
  const num = Number(value) * 100
  const sign = num > 0 ? '+' : ''
  return `${sign}${num.toFixed(digits)}%`
}

export function formatPortfolioTrackingNav(value) {
  if (value === null || value === undefined || Number.isNaN(Number(value))) return '--'
  return Number(value).toFixed(6)
}

export function formatPortfolioTrackingCurrency(value, exchange) {
  if (value === null || value === undefined || Number.isNaN(Number(value))) return '--'
  const prefix = String(exchange || '').toUpperCase() === 'HKEX' ? 'HK$' : '¥'
  return `${prefix}${Number(value).toLocaleString('zh-CN', {
    minimumFractionDigits: 2,
    maximumFractionDigits: 2,
  })}`
}

export function formatPortfolioTrackingShares(value) {
  if (value === null || value === undefined || Number.isNaN(Number(value))) return '--'
  return Number(value).toLocaleString('zh-CN', {
    minimumFractionDigits: 2,
    maximumFractionDigits: 2,
  })
}

export function getPortfolioTrackingPerformanceClass(value) {
  if (value === null || value === undefined || Number.isNaN(Number(value))) return 'text-foreground-dim'
  return Number(value) >= 0 ? 'text-negative' : 'text-positive'
}

export function getPortfolioTrackingStatusTone(status) {
  switch (String(status || '').toLowerCase()) {
    case 'completed':
      return 'border-positive/20 bg-positive/10 text-positive'
    case 'pending_close_price':
    case 'pending_open_price':
    case 'seeded':
      return 'border-amber-500/20 bg-amber-500/10 text-amber-700'
    case 'shortfall':
    case 'failed':
      return 'border-negative/20 bg-negative/10 text-negative'
    default:
      return 'border-border bg-background text-foreground-muted'
  }
}

export function buildPortfolioTrackingDetailHref(code, exchange) {
  const digits = String(code || '').replace(/\D/g, '')
  if (!digits) return ''
  const normalizedExchange = String(exchange || '').toUpperCase()
  if (normalizedExchange === 'HKEX') {
    return `/live-trading/${encodeURIComponent(`${digits.padStart(5, '0').slice(-5)}.HK`)}`
  }
  const code6 = digits.padStart(6, '0').slice(-6)
  const suffix = normalizedExchange === 'SSE' || code6.startsWith('6') || code6.startsWith('9') ? 'SH' : 'SZ'
  return `/live-trading/${encodeURIComponent(`${code6}.${suffix}`)}`
}

export function buildPortfolioTrackingMarketSections(items = []) {
  const grouped = Object.values(PORTFOLIO_TRACKING_MARKETS).map((market) => ({
    market,
    items: items.filter((item) => String(item?.exchange || '').toUpperCase() === market.key),
  }))
  return grouped.filter((group) => group.items.length > 0)
}

export function normalizePortfolioTrackingDaily(items = []) {
  return [...items]
    .filter((item) => item?.trade_date)
    .map((item) => ({
      ...item,
      trade_date: formatPortfolioTrackingDate(item.trade_date),
      nav: Number(item.nav || 0),
      total_assets: Number(item.total_assets || 0),
      daily_return: Number(item.daily_return || 0),
      total_return: Number(item.total_return || 0),
    }))
    .sort((left, right) => String(left.trade_date).localeCompare(String(right.trade_date)))
}

export function buildPortfolioTrackingChart(items = [], width = 720, height = 220, padding = 24) {
  const series = normalizePortfolioTrackingDaily(items)
  if (!series.length) {
    return {
      path: '',
      baselineY: height / 2,
      points: [],
      min: 0,
      max: 1,
    }
  }
  const values = series.map((item) => Number(item.nav || 0)).filter((value) => Number.isFinite(value) && value > 0)
  const min = Math.min(...values, 1)
  const max = Math.max(...values, 1)
  const range = Math.max(max - min, 0.001)
  const innerWidth = Math.max(width - padding * 2, 1)
  const innerHeight = Math.max(height - padding * 2, 1)
  const points = series.map((item, index) => {
    const x = padding + (innerWidth * index) / Math.max(series.length - 1, 1)
    const y = padding + innerHeight - ((item.nav - min) / range) * innerHeight
    return {
      ...item,
      x,
      y,
    }
  })
  return {
    path: points.map((point, index) => `${index === 0 ? 'M' : 'L'} ${point.x.toFixed(2)} ${point.y.toFixed(2)}`).join(' '),
    baselineY: padding + innerHeight - ((1 - min) / range) * innerHeight,
    points,
    min,
    max,
  }
}
