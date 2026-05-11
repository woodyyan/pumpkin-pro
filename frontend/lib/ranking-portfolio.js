export function formatRankingPortfolioPercent(value, digits = 2) {
  if (value === null || value === undefined || Number.isNaN(Number(value))) return '--'
  const num = Number(value)
  const sign = num > 0 ? '+' : ''
  return `${sign}${num.toFixed(digits)}%`
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

export function formatRankingPortfolioCode(code, exchange) {
  if (exchange === 'HKEX') return String(code || '').padStart(5, '0')
  return String(code || '')
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

