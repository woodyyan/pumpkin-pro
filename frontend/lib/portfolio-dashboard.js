import { requestJson } from './api'

/**
 * 组合控制台（Portfolio Dashboard）数据访问层
 */

export async function fetchPortfolioDashboard(query = {}) {
  const qs = new URLSearchParams()
  if (query.scope) qs.set('scope', query.scope)
  if (query.sort_by) qs.set('sort_by', query.sort_by)
  if (query.sort_order) qs.set('sort_order', query.sort_order)
  if (query.pnl_filter) qs.set('pnl_filter', query.pnl_filter)
  if (query.keyword) qs.set('keyword', query.keyword)
  if (query.curve_range) qs.set('curve_range', query.curve_range)

  const path = `/api/portfolio/dashboard${qs.toString() ? '?' + qs.toString() : ''}`
  return await requestJson(path, undefined, '加载组合控制台失败')
}

export async function fetchPortfolioEquityCurve(query = {}) {
  const qs = new URLSearchParams()
  if (query.scope) qs.set('scope', query.scope)
  if (query.range) qs.set('range', query.range)

  const path = `/api/portfolio/equity-curve${qs.toString() ? '?' + qs.toString() : ''}`
  return await requestJson(path, undefined, '加载资产曲线失败')
}

export async function fetchPortfolioRecentEvents(query = {}) {
  const qs = new URLSearchParams()
  if (query.scope) qs.set('scope', query.scope)
  if (query.keyword) qs.set('keyword', query.keyword)
  if (query.limit) qs.set('limit', String(query.limit))
  if (query.offset) qs.set('offset', String(query.offset))

  const path = `/api/portfolio/events/recent${qs.toString() ? '?' + qs.toString() : ''}`
  return await requestJson(path, undefined, '加载最近交易记录失败')
}

export async function fetchPortfolioAllocation(query = {}) {
  const qs = new URLSearchParams()
  if (query.scope) qs.set('scope', query.scope)
  if (query.keyword) qs.set('keyword', query.keyword)
  if (query.limit) qs.set('limit', String(query.limit))

  const path = `/api/portfolio/allocation${qs.toString() ? '?' + qs.toString() : ''}`
  return await requestJson(path, undefined, '加载持仓分布失败')
}

export async function fetchPortfolioAIContext(scope) {
  const qs = new URLSearchParams()
  if (scope) qs.set('scope', scope)
  const path = `/api/portfolio/ai-context${qs.toString() ? '?' + qs.toString() : ''}`
  return await requestJson(path, undefined, '加载 AI 分析上下文失败')
}

export async function fetchPortfolioDetail(symbol) {
  const normalized = String(symbol || '').trim().toUpperCase()
  if (!normalized) {
    return { item: null, history_preview: [] }
  }
  return await requestJson(`/api/portfolio/${encodeURIComponent(normalized)}`, undefined, '加载持仓详情失败')
}

export async function fetchPortfolioEventTimeline(symbol, query = {}) {
  const normalized = String(symbol || '').trim().toUpperCase()
  if (!normalized) {
    return { items: [], next_cursor: '' }
  }
  const qs = new URLSearchParams()
  if (query.limit) qs.set('limit', String(query.limit))

  const path = `/api/portfolio/${encodeURIComponent(normalized)}/events${qs.toString() ? '?' + qs.toString() : ''}`
  return await requestJson(path, undefined, '加载持仓变动记录失败')
}

export async function createPortfolioEvent(symbol, payload) {
  const normalized = String(symbol || '').trim().toUpperCase()
  if (!normalized) {
    throw new Error('股票代码不能为空')
  }
  return await requestJson(`/api/portfolio/${encodeURIComponent(normalized)}/events`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  }, '保存持仓变动失败')
}

export async function undoPortfolioEvent(symbol, eventId) {
  const normalized = String(symbol || '').trim().toUpperCase()
  const normalizedEventId = String(eventId || '').trim()
  if (!normalized || !normalizedEventId) {
    throw new Error('缺少可撤销的持仓记录')
  }
  return await requestJson(`/api/portfolio/${encodeURIComponent(normalized)}/events/${encodeURIComponent(normalizedEventId)}/undo`, {
    method: 'POST',
  }, '撤销持仓变动失败')
}

export async function deletePortfolioHistory(symbol) {
  const normalized = String(symbol || '').trim().toUpperCase()
  if (!normalized) {
    throw new Error('缺少可删除的股票代码')
  }
  return await requestJson(`/api/portfolio/${encodeURIComponent(normalized)}`, {
    method: 'DELETE',
  }, '删除持仓历史失败')
}

export function buildPortfolioDeleteConfirmText(symbol) {
  return `DELETE ${String(symbol || '').trim().toUpperCase()}`
}

export function resolvePortfolioTradeSymbol(input, market = 'ASHARE') {
  const raw = String(input || '').trim().toUpperCase()
  if (!raw) return ''
  if (raw.includes('.')) return raw

  const compact = raw.replace(/\s+/g, '')
  if (market === 'HKEX') {
    if (/^\d{1,5}$/.test(compact)) {
      return `${compact.padStart(5, '0')}.HK`
    }
    return ''
  }

  if (/^\d{6}$/.test(compact)) {
    const exchangeSuffix = compact.startsWith('6') || compact.startsWith('9') ? '.SH' : '.SZ'
    return `${compact}${exchangeSuffix}`
  }
  return ''
}

export function inferPortfolioTradeMarket(symbol, fallback = 'ASHARE') {
  const normalized = String(symbol || '').trim().toUpperCase()
  if (normalized.endsWith('.HK')) return 'HKEX'
  return fallback === 'HKEX' ? 'HKEX' : 'ASHARE'
}

// ── 格式化工具 ──

const PNL_COLOR_POSITIVE = 'text-rose-400' // 涨/盈 → 红（中国市场惯例）
const PNL_COLOR_NEGATIVE = 'text-emerald-400' // 跌/亏 → 绿
const NEUTRAL_COLOR = 'text-white/55'

export function formatPnl(value, opts = {}) {
  if (typeof value !== 'number' || Number.isNaN(value)) return '--'
  const symbol = opts.symbol || ''
  const signed = opts.signed !== false
  const prefix = signed && value > 0 ? '+' : ''
  const abs = Math.abs(value)
  let formatted
  if (Math.abs(value) >= 10000) {
    formatted = `${symbol}${prefix}${abs.toLocaleString('zh-CN', { maximumFractionDigits: 0, minimumFractionDigits: 0 })}`
  } else {
    formatted = `${symbol}${prefix}${abs.toLocaleString('zh-CN', { maximumFractionDigits: 2, minimumFractionDigits: 2 })}`
  }
  if (!opts.hideCurrency && !symbol) {
    formatted = (value > 0 ? '+' : value < 0 ? '-' : '') + formatMoney(abs, opts.exchange)
  }
  return { text: formatted, color: value > 0 ? PNL_COLOR_POSITIVE : value < 0 ? PNL_COLOR_NEGATIVE : NEUTRAL_COLOR }
}

export function formatPnlPercent(value) {
  if (typeof value !== 'number' || Number.isNaN(value)) return { text: '--', color: NEUTRAL_COLOR }
  const pct = Math.abs(value) * 100
  const text = (value >= 0 ? '+' : '-') + pct.toFixed(2) + '%'
  const color = value > 0 ? PNL_COLOR_POSITIVE : value < 0 ? PNL_COLOR_NEGATIVE : NEUTRAL_COLOR
  return { text, color }
}

export function formatMoney(value, exchange) {
  if (typeof value !== 'number' || Number.isNaN(value)) return '--'
  const symbol = exchange === 'HKEX' ? 'HK$' : '¥'
  return `${symbol}${value.toLocaleString('zh-CN', { maximumFractionDigits: 2, minimumFractionDigits: 2 })}`
}

export function formatCompactNumber(value) {
  if (typeof value !== 'number' || Number.isNaN(value)) return '--'
  if (Math.abs(value) >= 1e8) return (value / 1e8).toFixed(2) + '亿'
  if (Math.abs(value) >= 1e4) return (value / 1e4).toFixed(2) + '万'
  return value.toLocaleString('zh-CN', { maximumFractionDigits: 2 })
}

export function scopeLabel(scope) {
  switch (scope) {
    case 'ASHARE': return 'A股'
    case 'HKEX': return '港股'
    default: return '全部'
  }
}

export function exchangeTag(exchange) {
  if (exchange === 'HKEX') {
    return <span className="inline-flex items-center px-1 rounded text-[10px] font-medium bg-blue-500/20 text-blue-300">HK</span>
  }
  return null
}

// ── 股票价格获取 ──

/**
 * 获取股票实时快照（含最新价）
 * GET /api/live/symbols/{symbol}/snapshot
 */
export async function fetchSymbolSnapshot(symbol) {
  const normalized = String(symbol || '').trim().toUpperCase()
  if (!normalized) return null
  try {
    const data = await requestJson(`/api/live/symbols/${encodeURIComponent(normalized)}/snapshot`, undefined, '加载行情快照失败')
    return data?.snapshot || null
  } catch {
    return null
  }
}

/**
 * 获取股票历史日线数据
 * GET /api/live/symbols/{symbol}/daily-bars?lookback_days={n}
 */
export async function fetchSymbolDailyBars(symbol, lookbackDays = 260) {
  const normalized = String(symbol || '').trim().toUpperCase()
  if (!normalized) return []
  try {
    const data = await requestJson(
      `/api/live/symbols/${encodeURIComponent(normalized)}/daily-bars?lookback_days=${lookbackDays}`,
      undefined,
      '加载历史日线失败'
    )
    return Array.isArray(data?.bars) ? data.bars : []
  } catch {
    return []
  }
}

/**
 * 根据交易日期获取对应价格
 * - 今天或过去日期：尝试从历史日线找到对应日期的收盘价
 * - 如果找不到（节假日等），返回 null 让用户输入
 */
export function findClosePriceByDate(bars, tradeDate) {
  if (!Array.isArray(bars) || !tradeDate) return null
  const target = String(tradeDate)
  const found = bars.find(bar => String(bar.date) === target)
  if (found && typeof found.close === 'number' && found.close > 0) {
    return Math.round(found.close * 100) / 100 // 保留 2 位小数
  }
  return null
}

/**
 * 获取组合风险指标
 * GET /api/portfolio/risk-metrics?scope={scope}
 */
export async function fetchPortfolioRiskMetrics(query = {}) {
  const qs = new URLSearchParams()
  if (query.scope) qs.set('scope', query.scope)

  const path = `/api/portfolio/risk-metrics${qs.toString() ? '?' + qs.toString() : ''}`
  return await requestJson(path, undefined, '加载风险指标失败')
}
