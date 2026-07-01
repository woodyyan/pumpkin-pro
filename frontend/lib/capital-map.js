export const CAPITAL_MAP_REFRESH_MS = 1800000

export function buildCapitalMapUrl() {
  return '/api/capital-map'
}

export function formatPercent(value, digits = 2) {
  const number = Number(value)
  if (!Number.isFinite(number)) return '--'
  return `${number > 0 ? '+' : ''}${number.toFixed(digits)}%`
}

export function formatNumber(value, digits = 0) {
  const number = Number(value)
  if (!Number.isFinite(number)) return '--'
  return new Intl.NumberFormat('zh-CN', { maximumFractionDigits: digits, minimumFractionDigits: digits }).format(number)
}

export function formatCompactNumber(value, digits = 1) {
  const number = Number(value)
  if (!Number.isFinite(number)) return '--'
  return new Intl.NumberFormat('zh-CN', { maximumFractionDigits: digits }).format(number)
}

export function formatBeijingTime(value) {
  if (!value) return '--'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return '--'
  return date.toLocaleString('zh-CN', {
    timeZone: 'Asia/Shanghai',
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
  })
}

export function changeClassName(value) {
  const number = Number(value)
  if (number > 0) return 'text-negative'
  if (number < 0) return 'text-positive'
  return 'text-foreground-muted'
}

export function changeColor(value) {
  const number = Number(value)
  if (number > 0) return '#dc2626'
  if (number < 0) return '#16a34a'
  return '#94a3b8'
}

export function buildStockDetailHref(stock) {
  const code = String(stock?.code || '').trim()
  const market = String(stock?.market || '').trim().toUpperCase()
  if (!code) return '/live-trading'
  if (market === 'SH') return `/live-trading/${code}.SH`
  if (market === 'SZ') return `/live-trading/${code}.SZ`
  if (market === 'BJ') return `/live-trading/${code}.BJ`
  return `/live-trading/${code}`
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
    neutral: dark ? '#9ca3af' : '#94a3b8',
    gold: dark ? '#f59e0b' : '#d97706',
    goldMuted: dark ? '#78350f' : '#fef3c7',
    blue: dark ? '#60a5fa' : '#2563eb',
    bar: dark ? '#334155' : '#cbd5e1',
    barBorder: dark ? '#475569' : '#94a3b8',
  }
}
