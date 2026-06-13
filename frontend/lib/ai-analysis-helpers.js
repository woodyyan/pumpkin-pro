import { requestJson } from './api'
import { isAuthRequiredError } from './auth-storage'
import { deriveAIAnalysisWaitState } from './ai-analysis-wait'
import { fetchSymbolDailyBars, fetchSymbolSnapshot } from './portfolio-dashboard'
import { buildAINewsContext } from './symbol-news-ui'

export const AI_ANALYSIS_MIN_QUERY_LEN = 2
export const AI_ANALYSIS_SEARCH_LIMIT = 8
export const AI_ANALYSIS_GLOBAL_HISTORY_PAGE_SIZE = 10
export const AI_ANALYSIS_DEFAULT_ERROR = '分析遇到问题，请稍后重试。如果反复出现，请联系客服'

export function normalizeSearchResults(results = []) {
  if (!Array.isArray(results)) return []
  return results
    .map((item) => {
      const code = String(item?.code || '').trim()
      const exchange = String(item?.exchange || '').trim().toUpperCase()
      if (!code) return null
      const isHK = exchange === 'HKEX'
      const paddedCode = isHK ? code.padStart(5, '0') : code.padStart(6, '0')
      const symbol = isHK
        ? `${paddedCode}.HK`
        : (paddedCode.startsWith('6') || paddedCode.startsWith('9'))
          ? `${paddedCode}.SH`
          : `${paddedCode}.SZ`
      return {
        symbol,
        symbolName: String(item?.name || '').trim() || symbol,
        code: paddedCode,
        exchange,
        market: isHK ? 'HK' : 'ASHARE',
        displayText: `${paddedCode} ${String(item?.name || '').trim()}`.trim(),
        matchType: '',
        raw: item,
      }
    })
    .filter(Boolean)
}

export async function searchAnalysisTargets(query, { limit = AI_ANALYSIS_SEARCH_LIMIT } = {}) {
  const text = String(query || '').trim()
  if (text.length < AI_ANALYSIS_MIN_QUERY_LEN) {
    return []
  }
  const data = await requestJson(`/api/search?q=${encodeURIComponent(text)}&limit=${limit}`)
  return normalizeSearchResults(data?.results || [])
}

export async function resolveAnalysisTarget(input, options = {}) {
  const items = await searchAnalysisTargets(input, options)
  const normalizedInput = String(input || '').trim().toUpperCase()
  const exact = items.find((item) => item.symbol.toUpperCase() === normalizedInput || item.code.toUpperCase() === normalizedInput)
  return {
    items,
    exact: exact || null,
    selected: exact || (items.length === 1 ? items[0] : null),
  }
}

export async function buildAIAnalysisContext({
  symbol,
  symbolName,
  exchange,
  snapshotPayload,
  lastUpdateAt,
  movingAveragePayload,
  fundamentalsItems,
  portfolioData,
  buildMarketOverview,
  fetchNewsContext,
  formatYiCurrency,
  formatYiAmount,
  formatYiShares,
}) {
  const isAShare = exchange === 'SSE' || exchange === 'SZSE'
  const snap = snapshotPayload?.snapshot
  if (!snap) {
    throw new Error('行情数据尚未加载完成，请稍后再试')
  }

  const symbolMeta = { symbol, name: symbolName || symbol, exchange, currency: isAShare ? 'CNY' : 'HKD' }
  const market = {
    price: snap.last_price ?? 0,
    change_pct: snap.change_rate ?? 0,
    volume: snap.volume ?? null,
    turnover_rate: snap.turnover_rate ?? null,
    open: snap.open ?? null,
    high: snap.high ?? null,
    low: snap.low ?? null,
    data_ts: lastUpdateAt || new Date().toISOString(),
  }

  let technical
  if (movingAveragePayload && Number(movingAveragePayload?.price_ref || 0) > 0) {
    technical = {
      ma5: movingAveragePayload.ma5 ?? 'N/A',
      ma20: movingAveragePayload.ma20 ?? 'N/A',
      ma60: movingAveragePayload.ma60 ?? 'N/A',
      ma200: movingAveragePayload.ma200 ?? 'N/A',
      ma_status: movingAveragePayload.status || 'N/A',
      rsi14: movingAveragePayload.rsi14 ?? 'N/A',
      rsi14_status: movingAveragePayload.rsi14_status || 'N/A',
      macd: movingAveragePayload.macd ?? 'N/A',
      macd_signal: movingAveragePayload.macd_signal ?? 'N/A',
      macd_histogram: movingAveragePayload.macd_histogram ?? 'N/A',
      bollinger_upper: movingAveragePayload.bollinger_upper ?? 'N/A',
      bollinger_middle: movingAveragePayload.bollinger_middle ?? 'N/A',
      bollinger_lower: movingAveragePayload.bollinger_lower ?? 'N/A',
      bollinger_bandwidth: movingAveragePayload.bollinger_bandwidth ?? 'N/A',
      bollinger_percent_b: movingAveragePayload.bollinger_percent_b ?? 'N/A',
      change_pct_60d: movingAveragePayload.change_pct_60d ?? 'N/A',
      volatility_20d: movingAveragePayload.volatility_20d ?? 'N/A',
      volume_ma5_to_ma20: movingAveragePayload.volume_ma5_to_ma20 ?? 'N/A',
      _valid: true,
    }
  } else {
    technical = { _valid: false }
  }

  let fundamentals
  if (fundamentalsItems && Object.keys(fundamentalsItems).length > 3) {
    fundamentals = {
      market_cap: fundamentalsItems.market_cap ?? 'N/A',
      market_cap_text: formatYiCurrency(fundamentalsItems.market_cap, isAShare ? '¥' : 'HK$'),
      pe_ttm: fundamentalsItems.pe_ttm ?? 'N/A',
      pe_unavailable: !fundamentalsItems.pe_ttm || Number(fundamentalsItems.pe_ttm) <= 0,
      pb: fundamentalsItems.pb_ttm ?? 'N/A',
      pb_unavailable: !fundamentalsItems.pb_ttm || Number(fundamentalsItems.pb_ttm) <= 0,
      peg: fundamentalsItems.peg ?? 'N/A',
      peg_unavailable: fundamentalsItems.peg == null || Number(fundamentalsItems.peg) <= 0,
      dividend_yield: fundamentalsItems.dividend_yield ?? 'N/A',
      div_yield_unavailable: !fundamentalsItems.dividend_yield || Number(fundamentalsItems.dividend_yield) < 0,
      net_profit: fundamentalsItems.net_profit_fy ?? 'N/A',
      net_profit_text: formatYiAmount(fundamentalsItems.net_profit_fy, isAShare ? '¥' : 'HK$'),
      revenue: fundamentalsItems.revenue_fy ?? 'N/A',
      revenue_text: formatYiAmount(fundamentalsItems.revenue_fy, isAShare ? '¥' : 'HK$'),
      shares_outstanding: fundamentalsItems.float_shares ?? 'N/A',
      shares_outstanding_text: formatYiShares(fundamentalsItems.float_shares),
      _valid: true,
    }
  } else {
    fundamentals = { _valid: false }
  }

  const marketOverview = typeof buildMarketOverview === 'function' ? await buildMarketOverview(exchange) : { _valid: false }
  const newsContext = typeof fetchNewsContext === 'function' ? await fetchNewsContext(symbol) : { payload: { _valid: false }, state: 'idle' }

  let portfolio = { has_position: false }
  if (portfolioData && portfolioData.shares > 0) {
    const pnlPct = snap?.last_price && portfolioData.avg_cost_price > 0
      ? ((snap.last_price / portfolioData.avg_cost_price) - 1) * 100
      : 0
    const pnlAmount = snap?.last_price && portfolioData.avg_cost_price > 0
      ? (snap.last_price - portfolioData.avg_cost_price) * portfolioData.shares
      : 0
    portfolio = {
      has_position: true,
      shares: portfolioData.shares,
      avg_cost_price: portfolioData.avg_cost_price || 0,
      total_cost_amount: portfolioData.total_cost_amount || 0,
      buy_date: portfolioData.buy_date || '',
      cost_method: portfolioData.cost_method || 'weighted_avg',
      cost_source: portfolioData.cost_source || 'system',
      last_trade_at: portfolioData.last_trade_at || '',
      unrealized_pnl: pnlAmount,
      unrealized_pnl_text: formatYiCurrency(pnlAmount, isAShare ? '¥' : 'HK$'),
      unrealized_pnl_pct: pnlPct,
    }
  }

  return {
    payload: {
      symbol_meta: symbolMeta,
      market,
      technical,
      fundamentals,
      market_overview: marketOverview,
      portfolio,
      news_context: newsContext.payload,
    },
    newsState: newsContext.state,
  }
}

export function deriveMovingAveragePayloadFromBars(bars = []) {
  const safeBars = Array.isArray(bars) ? bars : []
  if (!safeBars.length) return null

  const recent = safeBars.slice(-60)
  const closes = recent.map((item) => Number(item?.close) || 0).filter((value) => value > 0)
  if (!closes.length) return null

  const sum = (arr) => arr.reduce((acc, value) => acc + value, 0)
  const calcMA = (days) => {
    const subset = closes.slice(-days)
    return subset.length ? Number((sum(subset) / subset.length).toFixed(3)) : null
  }

  return {
    price_ref: closes.at(-1) || 0,
    ma5: calcMA(5),
    ma20: calcMA(20),
    ma60: calcMA(60),
    status: 'derived',
  }
}

export async function fetchAIAnalysisNewsContext(symbol) {
  try {
    const [newsSummaryData, newsListData] = await Promise.all([
      requestJson(`/api/live/symbols/${encodeURIComponent(symbol)}/news/summary`),
      requestJson(`/api/live/symbols/${encodeURIComponent(symbol)}/news?limit=8`),
    ])
    const payload = buildAINewsContext({
      summary: newsSummaryData?.summary || newsListData?.summary || null,
      items: newsListData?.items || [],
      maxItems: 6,
    })
    return { payload, state: payload?._valid ? 'ready' : 'empty' }
  } catch {
    return { payload: { _valid: false }, state: 'error' }
  }
}

export async function runAIAnalysisRequest({ symbol, payload }) {
  return requestJson(`/api/live/symbols/${encodeURIComponent(symbol)}/ai-analysis`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  })
}

export async function fetchGlobalAIAnalysisHistory({ page = 1, pageSize = AI_ANALYSIS_GLOBAL_HISTORY_PAGE_SIZE } = {}) {
  return requestJson(`/api/ai-analysis/history?page=${page}&page_size=${pageSize}`)
}

export async function loadAIAnalysisDependencies(target, { isLoggedIn = false } = {}) {
  const symbol = target?.symbol
  if (!symbol) {
    throw new Error('缺少股票标识，无法准备分析数据')
  }

  const [snapshot, dailyBars, fundamentalsData, portfolioRes] = await Promise.all([
    fetchSymbolSnapshot(symbol),
    fetchSymbolDailyBars(symbol, 240),
    requestJson(`/api/live/symbols/${encodeURIComponent(symbol)}/fundamentals`).catch(() => null),
    isLoggedIn ? requestJson(`/api/portfolio/${encodeURIComponent(symbol)}`).catch(() => null) : Promise.resolve(null),
  ])

  const bars = Array.isArray(dailyBars) ? dailyBars : []
  const movingAveragePayload = deriveMovingAveragePayloadFromBars(bars)
  const fundamentalsItems = fundamentalsData?.items || fundamentalsData?.fundamentals || null
  const portfolioData = portfolioRes?.item || null
  const lastUpdateAt = new Date().toISOString()

  return {
    snapshotPayload: snapshot ? { snapshot } : null,
    movingAveragePayload,
    fundamentalsItems,
    portfolioData,
    lastUpdateAt,
  }
}

export async function fetchSymbolAIAnalysisHistory(symbol, { limit = 10 } = {}) {
  return requestJson(`/api/live/symbols/${encodeURIComponent(symbol)}/analysis-history?limit=${limit}`)
}

export async function fetchAIAnalysisHistoryDetail(symbol, id) {
  return requestJson(`/api/live/symbols/${encodeURIComponent(symbol)}/analysis-history?id=${id}`)
}

export async function deleteAIAnalysisHistory(symbol, id) {
  return requestJson(`/api/live/symbols/${encodeURIComponent(symbol)}/analysis-history?id=${id}`, {
    method: 'DELETE',
  })
}

export function createAIAnalysisInitialState() {
  return {
    analyzing: false,
    result: null,
    error: '',
    showPanel: false,
    waitStartedAt: 0,
    waitElapsedSec: 0,
    newsContextState: 'idle',
    notifPromptVisible: false,
    requestId: 0,
  }
}

export function deriveControllerWaitState(state, { hasPosition = false } = {}) {
  return deriveAIAnalysisWaitState(state.waitElapsedSec, { hasPosition, newsState: state.newsContextState })
}

export function maybePromptNotification() {
  return typeof window !== 'undefined' && typeof Notification !== 'undefined' && Notification.permission === 'default'
}

export function mapAIAnalysisError(err) {
  if (isAuthRequiredError(err)) return '登录已过期，请重新登录后重试'
  const message = String(err?.message || '')
  if (message.includes('429') || message.includes('Too Many')) return 'AI 分析次数已达上限，请 1 小时后再试，或联系管理员提升限额'
  if (message.includes('timeout') || message.includes('Timeout')) return '分析响应较慢（已自动重试），该股票数据量较大，请稍后再试'
  if (message) return message
  return AI_ANALYSIS_DEFAULT_ERROR
}
