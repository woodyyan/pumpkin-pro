import { requestJson } from './api'

export async function fetchAIPickerMeta(market = 'ASHARE') {
  return requestJson(`/api/ai/picker/meta?market=${encodeURIComponent(market)}`, undefined, '加载 AI 选股状态失败')
}

export async function fetchDailyAIPicks(market = 'ASHARE') {
  return requestJson(`/api/ai/picker/daily?market=${encodeURIComponent(market)}`, undefined, '加载今日 AI 选股失败')
}

export async function generateAIPicks({ market = 'ASHARE', direction = '', refresh = true } = {}) {
  return requestJson('/api/ai/picker', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ market, direction, refresh }),
  }, '生成 AI 选股失败')
}

export function formatPct(value) {
  const num = Number(value)
  if (!Number.isFinite(num)) return '--'
  return `${num}%`
}

export function formatPrice(value, currency = 'CNY') {
  const num = Number(value)
  if (!Number.isFinite(num)) return '--'
  return `${currency === 'HKD' ? 'HK$' : '¥'}${num.toFixed(2)}`
}

export function convictionTone(level) {
  if (level === 'high') return 'bg-emerald-500/15 text-emerald-300 border-emerald-500/25'
  if (level === 'medium') return 'bg-blue-500/15 text-blue-300 border-blue-500/25'
  return 'bg-amber-500/15 text-amber-300 border-amber-500/25'
}
