import { describe, it } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const helperSource = readFileSync(new URL('../ai-analysis-helpers.js', import.meta.url), 'utf8')

function deriveMovingAveragePayloadFromBars(bars = []) {
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

async function loadAIAnalysisDependenciesForTest({ symbol, isLoggedIn = false, snapshot = null, dailyBars = [], fundamentalsData = null, portfolioRes = null }) {
  if (!symbol) {
    throw new Error('缺少股票标识，无法准备分析数据')
  }

  const bars = Array.isArray(dailyBars) ? dailyBars : []
  const movingAveragePayload = deriveMovingAveragePayloadFromBars(bars)
  const fundamentalsItems = fundamentalsData?.items || fundamentalsData?.fundamentals || null
  const portfolioData = isLoggedIn ? (portfolioRes?.item || null) : null
  const lastUpdateAt = new Date().toISOString()

  return {
    snapshotPayload: snapshot ? { snapshot } : null,
    movingAveragePayload,
    fundamentalsItems,
    portfolioData,
    lastUpdateAt,
  }
}

function formatYiCurrency(value, prefix = '¥') {
  if (typeof value !== 'number' || !Number.isFinite(value)) return '--'
  return `${prefix}${(value / 1e8).toFixed(2)}亿`
}

function formatYiAmount(value, prefix = '¥') {
  if (typeof value !== 'number' || !Number.isFinite(value)) return '--'
  return `${prefix}${(value / 1e8).toFixed(2)}亿`
}

function formatYiShares(value) {
  if (typeof value !== 'number' || !Number.isFinite(value)) return '--'
  return `${(value / 1e8).toFixed(2)}亿股`
}

async function buildAIAnalysisContextForTest({
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

describe('ai-analysis helper implementation contract', () => {
  it('keeps the fixed dependency normalization in source', () => {
    assert.match(helperSource, /const \[snapshot, dailyBars, fundamentalsData, portfolioRes\] = await Promise\.all\(\[/)
    assert.match(helperSource, /const bars = Array\.isArray\(dailyBars\) \? dailyBars : \[\]/)
    assert.match(helperSource, /snapshotPayload: snapshot \? \{ snapshot \} : null/)
    assert.doesNotMatch(helperSource, /dailyBarsData\?\.bars/)
  })
})

describe('loadAIAnalysisDependencies normalization', () => {
  it('wraps snapshot data and derives moving averages from daily bar arrays', async () => {
    const snapshot = { last_price: 18.52, change_rate: 2.31, volume: 123456, turnover_rate: 3.2 }
    const dailyBars = [
      { close: 10 },
      { close: 11 },
      { close: 12 },
      { close: 13 },
      { close: 14 },
      { close: 15 },
    ]
    const fundamentalsData = { items: { market_cap: 12000000000, pe_ttm: 18.5, pb_ttm: 3.2, net_profit_fy: 1000000000, revenue_fy: 5000000000, float_shares: 900000000 } }
    const portfolioRes = { item: { shares: 1000, avg_cost_price: 16.8 } }

    const result = await loadAIAnalysisDependenciesForTest({ symbol: '000001.SZ', isLoggedIn: true, snapshot, dailyBars, fundamentalsData, portfolioRes })

    assert.deepEqual(result.snapshotPayload, { snapshot })
    assert.equal(result.movingAveragePayload?.price_ref, 15)
    assert.equal(result.movingAveragePayload?.ma5, 13)
    assert.deepEqual(result.fundamentalsItems, fundamentalsData.items)
    assert.deepEqual(result.portfolioData, portfolioRes.item)
    assert.match(result.lastUpdateAt, /^\d{4}-\d{2}-\d{2}T/)
  })

  it('keeps snapshotPayload null and skips portfolio data when not logged in', async () => {
    const result = await loadAIAnalysisDependenciesForTest({
      symbol: '00700.HK',
      isLoggedIn: false,
      snapshot: null,
      dailyBars: [{ close: 20 }, { close: 21 }],
      fundamentalsData: { items: { market_cap: 5000000000, pe_ttm: 12, pb_ttm: 1.8, net_profit_fy: 600000000, revenue_fy: 2400000000 } },
      portfolioRes: { item: { shares: 88, avg_cost_price: 320 } },
    })

    assert.equal(result.snapshotPayload, null)
    assert.equal(result.portfolioData, null)
    assert.equal(result.movingAveragePayload?.price_ref, 21)
  })
})

describe('buildAIAnalysisContext compatibility', () => {
  it('accepts normalized dependency payload and builds a complete context', async () => {
    const context = await buildAIAnalysisContextForTest({
      symbol: '000001.SZ',
      symbolName: '平安银行',
      exchange: 'SZSE',
      snapshotPayload: {
        snapshot: {
          last_price: 15,
          change_rate: 1.5,
          volume: 123456,
          turnover_rate: 2.8,
          open: 14.5,
          high: 15.3,
          low: 14.2,
        },
      },
      lastUpdateAt: '2026-06-13T08:00:00.000Z',
      movingAveragePayload: deriveMovingAveragePayloadFromBars([{ close: 10 }, { close: 11 }, { close: 12 }, { close: 13 }, { close: 14 }, { close: 15 }]),
      fundamentalsItems: {
        market_cap: 120000000000,
        pe_ttm: 18.5,
        pb_ttm: 1.2,
        peg: 0.9,
        dividend_yield: 2.1,
        net_profit_fy: 20000000000,
        revenue_fy: 80000000000,
        float_shares: 10000000000,
      },
      portfolioData: {
        shares: 1000,
        avg_cost_price: 12,
        total_cost_amount: 12000,
        buy_date: '2026-01-01',
      },
      buildMarketOverview: async () => ({ _valid: true, trend_summary: '多数上涨（2/3）', indexes: [] }),
      fetchNewsContext: async () => ({ state: 'ready', payload: { _valid: true, items: [] } }),
    })

    assert.equal(context.payload.market.price, 15)
    assert.equal(context.payload.technical._valid, true)
    assert.equal(context.payload.fundamentals._valid, true)
    assert.equal(context.payload.portfolio.has_position, true)
    assert.equal(context.newsState, 'ready')
  })

  it('throws the loading error when snapshot is still missing', async () => {
    await assert.rejects(
      () => buildAIAnalysisContextForTest({
        symbol: '000001.SZ',
        symbolName: '平安银行',
        exchange: 'SZSE',
        snapshotPayload: null,
        lastUpdateAt: '2026-06-13T08:00:00.000Z',
        movingAveragePayload: null,
        fundamentalsItems: null,
        portfolioData: null,
        buildMarketOverview: async () => ({ _valid: false }),
        fetchNewsContext: async () => ({ state: 'idle', payload: { _valid: false } }),
      }),
      /行情数据尚未加载完成/
    )
  })
})
