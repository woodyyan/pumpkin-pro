export const NEWS_KLINE_REFRESH_MS = 30 * 60 * 1000
export const NEWS_KLINE_DEFAULT_DAYS = 500
export const NEWS_KLINE_DEFAULT_PAGES = 3

export function buildNewsKlineUrl({ symbol, days = NEWS_KLINE_DEFAULT_DAYS, pages = NEWS_KLINE_DEFAULT_PAGES, force = false } = {}) {
  const params = new URLSearchParams()
  if (symbol) params.set('symbol', symbol)
  params.set('days', String(days))
  params.set('pages', String(pages))
  if (force) params.set('force', '1')
  return `/api/live/news-kline?${params.toString()}`
}

export function symbolFromSearchResult(item) {
  if (!item) return ''
  const code = String(item.code || item.c || '').trim()
  const exchange = String(item.exchange || item.e || '').trim().toUpperCase()
  if (!code) return ''
  if (exchange === 'HKEX') return `${code.padStart(5, '0')}.HK`
  if (exchange === 'SSE') return `${code.padStart(6, '0')}.SH`
  if (exchange === 'BJSE') return `${code.padStart(6, '0')}.BJ`
  return `${code.padStart(6, '0')}.SZ`
}

export function formatPercent(value, digits = 1) {
  const number = Number(value)
  if (!Number.isFinite(number)) return '--'
  return `${number > 0 ? '+' : ''}${(number * 100).toFixed(digits)}%`
}

export function formatNumber(value, digits = 2) {
  const number = Number(value)
  if (!Number.isFinite(number)) return '--'
  return new Intl.NumberFormat('zh-CN', { maximumFractionDigits: digits, minimumFractionDigits: digits }).format(number)
}

export function formatBeijingTime(value) {
  if (!value) return '--'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return String(value)
  return date.toLocaleString('zh-CN', {
    timeZone: 'Asia/Shanghai',
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  })
}

export function changeClassName(value) {
  const number = Number(value)
  if (number > 0) return 'text-negative'
  if (number < 0) return 'text-positive'
  return 'text-foreground-muted'
}

export function chartPalette(resolvedTheme) {
  const dark = resolvedTheme === 'dark'
  return {
    axis: dark ? '#9ca3af' : '#64748b',
    split: dark ? 'rgba(255,255,255,0.08)' : 'rgba(15,23,42,0.10)',
    tooltipBg: dark ? 'rgba(22,22,24,0.96)' : 'rgba(255,255,255,0.98)',
    tooltipText: dark ? '#ededed' : '#1a1a2e',
    tooltipBorder: dark ? 'rgba(255,255,255,0.12)' : 'rgba(15,23,42,0.12)',
    red: dark ? '#ef4444' : '#dc2626',
    green: dark ? '#22c55e' : '#16a34a',
    blue: dark ? '#60a5fa' : '#2563eb',
    purple: dark ? '#a78bfa' : '#8b5cf6',
    amber: dark ? '#fbbf24' : '#f59e0b',
    teal: dark ? '#2dd4bf' : '#14b8a6',
    neutral: dark ? '#9ca3af' : '#94a3b8',
    card: dark ? '#161618' : '#ffffff',
  }
}

export function buildInsightText(report) {
  const stats = Array.isArray(report?.STATS) ? report.STATS : []
  const meta = report?.META || {}
  if (!stats.length) return '暂无足够事件样本，建议扩大观察窗口或稍后重试。'
  const top = stats[0]
  const name = meta.name || meta.symbol || '该股票'
  const direction = Number(top.avg_3d) >= 0 ? '上涨' : '下跌'
  return `对 ${name} 股价解释力最强的是「${top.category}」类事件（${top.count} 条），后 3 日平均收益 ${formatPercent(top.avg_3d)}，胜率 ${formatPercent(top.win_3d, 0)}，事件出现后短期平均${direction}。`
}

export function normalizeRange(range) {
  return ['1M', '3M', '6M', '1Y', 'ALL'].includes(range) ? range : '1Y'
}

export function filterKlineByRange(kline, range) {
  const rows = Array.isArray(kline) ? kline : []
  const normalized = normalizeRange(range)
  if (!rows.length || normalized === 'ALL') return rows
  const endDate = rows[rows.length - 1]?.date
  if (!endDate) return rows
  const start = new Date(`${endDate}T00:00:00+08:00`)
  if (normalized === '1M') start.setMonth(start.getMonth() - 1)
  if (normalized === '3M') start.setMonth(start.getMonth() - 3)
  if (normalized === '6M') start.setMonth(start.getMonth() - 6)
  if (normalized === '1Y') start.setFullYear(start.getFullYear() - 1)
  const startText = start.toISOString().slice(0, 10)
  return rows.filter((item) => String(item.date || '') >= startText)
}
