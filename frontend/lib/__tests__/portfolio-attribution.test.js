import { beforeEach, describe, it } from 'node:test'
import assert from 'node:assert/strict'

import {
  attributionToneClass,
  buildAttributionDetailRequestKeys,
  buildAttributionHeroBadges,
  buildAttributionWaterfallSeries,
  buildPortfolioAttributionPath,
  buildPortfolioAttributionQuery,
  createAttributionDetailSectionsState,
  fetchPortfolioAttributionSummary,
  formatAttributionMoney,
  formatAttributionPercent,
  formatAttributionScopeLabel,
  pickAttributionMarketSnapshot,
  pickAttributionSectorHighlights,
  pickAttributionStockHighlights,
  pickAttributionTradingHighlights,
  resolveAttributionActiveScope,
} from '../portfolio-attribution.js'

function makeJsonResponse(body, status = 200) {
  const text = JSON.stringify(body)
  return {
    ok: status >= 200 && status < 300,
    status,
    text: async () => text,
  }
}

const SAMPLE_SUMMARY = {
  headline: '过去30天，组合盈利主要来自选股超额，调仓贡献有限。',
  waterfall_groups: [
    {
      scope: 'ASHARE',
      scope_label: 'A股',
      currency_symbol: '¥',
      items: [
        { key: 'market', label: '市场贡献', amount: 3200 },
        { key: 'selection', label: '选股超额', amount: 6800 },
        { key: 'trading', label: '调仓贡献', amount: 900 },
        { key: 'fee', label: '手续费拖累', amount: -280 },
        { key: 'total_pnl', label: '总收益', type: 'total', amount: 10620 },
      ],
    },
    {
      scope: 'HKEX',
      scope_label: '港股',
      currency_symbol: 'HK$',
      items: [
        { key: 'market', label: '市场贡献', amount: -1200 },
        { key: 'selection', label: '选股超额', amount: 300 },
        { key: 'trading', label: '调仓贡献', amount: 50 },
        { key: 'fee', label: '手续费拖累', amount: -80 },
        { key: 'total_pnl', label: '总收益', type: 'total', amount: -930 },
      ],
    },
  ],
}

const SAMPLE_STOCKS = {
  positive_groups: [
    {
      scope: 'ASHARE',
      scope_label: 'A股',
      currency_symbol: '¥',
      items: [
        { symbol: '300750', name: '宁德时代', driver_label: '选股驱动', total_pnl_amount: 4200, contribution_ratio: 0.4 },
        { symbol: '600519', name: '贵州茅台', driver_label: '趋势驱动', total_pnl_amount: 1800, contribution_ratio: 0.17 },
      ],
    },
  ],
  negative_groups: [
    {
      scope: 'ASHARE',
      scope_label: 'A股',
      currency_symbol: '¥',
      items: [
        { symbol: '000001', name: '平安银行', driver_label: '市场拖累', total_pnl_amount: -900, contribution_ratio: -0.08 },
      ],
    },
  ],
}

const SAMPLE_TRADING = {
  groups: [
    {
      scope: 'ASHARE',
      scope_label: 'A股',
      currency_symbol: '¥',
      timeline: [
        { event_id: '1', symbol: '300750', trade_date: '2026-04-18', timing_effect_amount: 860, realized_pnl_amount: 1200 },
        { event_id: '2', symbol: '600519', trade_date: '2026-04-09', timing_effect_amount: -430, realized_pnl_amount: -150 },
        { event_id: '3', symbol: '000333', trade_date: '2026-04-03', timing_effect_amount: 300, realized_pnl_amount: 420 },
      ],
    },
  ],
}

const SAMPLE_MARKET = {
  groups: [
    {
      scope: 'ASHARE',
      scope_label: 'A股',
      benchmark_code: '000001.SH',
      benchmark_name: '上证指数',
      portfolio_return_pct: 0.082,
      benchmark_return_pct: 0.051,
      excess_return_pct: 0.031,
      market_contribution_amount: 3200,
      selection_contribution_amount: 6800,
      currency_symbol: '¥',
      series: [
        { date: '2026-04-01', portfolio_nav: 1, benchmark_nav: 1 },
        { date: '2026-04-15', portfolio_nav: 1.08, benchmark_nav: 1.05 },
      ],
    },
  ],
}

const SAMPLE_SECTORS = {
  groups: [
    {
      scope: 'ASHARE',
      scope_label: 'A股',
      currency_symbol: '¥',
      items: [
        { sector_name: '新能源', stock_count: 4, driver_label: '龙头上涨', total_pnl_amount: 2600, contribution_ratio: 0.24 },
        { sector_name: '白酒', stock_count: 2, driver_label: '核心持仓贡献', total_pnl_amount: 1500, contribution_ratio: 0.14 },
        { sector_name: '银行', stock_count: 3, driver_label: '防守仓拖累', total_pnl_amount: -700, contribution_ratio: -0.07 },
      ],
    },
  ],
}

describe('portfolio attribution helpers', () => {
  beforeEach(() => {
    global.window = {
      localStorage: {
        getItem() { return null },
        setItem() {},
        removeItem() {},
      },
    }
    global.fetch = async () => makeJsonResponse({ ok: true })
  })

  it('buildPortfolioAttributionQuery uses URLSearchParams-compatible keys', () => {
    const query = buildPortfolioAttributionQuery({
      scope: 'HKEX',
      range: '90D',
      start_date: '2026-01-01',
      end_date: '2026-03-31',
      limit: 8,
      sort_by: 'total_pnl',
      refresh: true,
      include_unclassified: true,
      timeline_limit: 12,
    })

    assert.equal(
      query,
      'scope=HKEX&range=90D&start_date=2026-01-01&end_date=2026-03-31&limit=8&sort_by=total_pnl&refresh=true&include_unclassified=true&timeline_limit=12'
    )
  })

  it('defaults market and sector detail sections to expanded', () => {
    const state = createAttributionDetailSectionsState()

    assert.equal(state.marketExpanded, true)
    assert.equal(state.sectorExpanded, true)
  })

  it('builds detail request keys from open sections', () => {
    assert.deepEqual(buildAttributionDetailRequestKeys({ detailOpen: false }), [])
    assert.deepEqual(
      buildAttributionDetailRequestKeys({ detailOpen: true, marketExpanded: true, sectorExpanded: true }),
      ['stocks', 'trading', 'market', 'sectors']
    )
    assert.deepEqual(
      buildAttributionDetailRequestKeys({ detailOpen: true, marketExpanded: true, sectorExpanded: false }),
      ['stocks', 'trading', 'market']
    )
  })

  it('preserves active scope when refreshed data still contains it', () => {
    const scopes = [
      { scope: 'ASHARE', label: 'A股' },
      { scope: 'HKEX', label: '港股' },
    ]

    assert.equal(resolveAttributionActiveScope(scopes, 'HKEX', 'ASHARE'), 'HKEX')
    assert.equal(resolveAttributionActiveScope(scopes, 'US', 'ASHARE'), 'ASHARE')
    assert.equal(resolveAttributionActiveScope(scopes, null, 'HKEX'), 'HKEX')
  })

  it('buildPortfolioAttributionPath omits question mark when query is empty', () => {
    assert.equal(buildPortfolioAttributionPath('summary', {}), '/api/portfolio/attribution/summary')
  })

  it('fetchPortfolioAttributionSummary requests the expected endpoint', async () => {
    let capturedUrl = ''
    global.fetch = async (input) => {
      capturedUrl = String(input)
      return makeJsonResponse({ headline: 'mock summary' })
    }

    const result = await fetchPortfolioAttributionSummary({ scope: 'ASHARE', range: '30D', limit: 5 })

    assert.equal(capturedUrl, '/api/portfolio/attribution/summary?scope=ASHARE&range=30D&limit=5')
    assert.deepEqual(result, { headline: 'mock summary' })
  })

  it('formats money with chinese sign conventions', () => {
    assert.equal(formatAttributionMoney(12345.67, '¥', { compact: true }), '+¥1.23万')
    assert.equal(formatAttributionMoney(-88.5, 'HK$'), '-HK$88.50')
  })

  it('formats percent and scope labels', () => {
    assert.equal(formatAttributionPercent(0.1234), '+12.3%')
    assert.equal(formatAttributionPercent(-0.056, 2), '-5.60%')
    assert.equal(formatAttributionScopeLabel('HKEX'), '港股')
    assert.equal(formatAttributionScopeLabel('ASHARE'), 'A股')
  })

  it('maps positive/negative values to chinese-market tone classes', () => {
    assert.equal(attributionToneClass(1.2), 'text-rose-400')
    assert.equal(attributionToneClass(-1.2), 'text-emerald-400')
    assert.equal(attributionToneClass(0), 'text-white/82')
  })

  it('builds hero badges from the selected scope waterfall', () => {
    const hero = buildAttributionHeroBadges(SAMPLE_SUMMARY, 'ASHARE')

    assert.equal(hero.activeScope, 'ASHARE')
    assert.equal(hero.totalAmount, 10620)
    assert.equal(hero.primaryDriver?.label, '选股超额')
    assert.equal(hero.biggestDrag?.label, '手续费拖累')
    assert.equal(hero.badges[0].value, '+¥1.06万')
  })

  it('builds waterfall series with running totals and total bar', () => {
    const series = buildAttributionWaterfallSeries(SAMPLE_SUMMARY.waterfall_groups[0])

    assert.equal(series[0].start, 0)
    assert.equal(series[0].end, 3200)
    assert.equal(series[1].start, 3200)
    assert.equal(series[1].end, 10000)
    assert.equal(series[4].isTotal, true)
    assert.equal(series[4].start, 0)
    assert.equal(series[4].end, 10620)
  })

  it('picks stock highlights from positive and negative groups', () => {
    const highlights = pickAttributionStockHighlights(SAMPLE_STOCKS, 'ASHARE', 1)

    assert.equal(highlights.positive.length, 1)
    assert.equal(highlights.positive[0].symbol, '300750')
    assert.equal(highlights.negative[0].symbol, '000001')
  })

  it('picks the most important positive and negative trading events', () => {
    const highlights = pickAttributionTradingHighlights(SAMPLE_TRADING, 'ASHARE', 1)

    assert.equal(highlights.positive.length, 1)
    assert.equal(highlights.positive[0].event_id, '1')
    assert.equal(highlights.negative.length, 1)
    assert.equal(highlights.negative[0].event_id, '2')
  })

  it('returns the active market snapshot for the chosen scope', () => {
    const snapshot = pickAttributionMarketSnapshot(SAMPLE_MARKET, 'ASHARE')

    assert.equal(snapshot.benchmark_name, '上证指数')
    assert.equal(snapshot.excess_return_pct, 0.031)
    assert.equal(snapshot.series.length, 2)
  })

  it('reduces sector payload into top positive and negative industries', () => {
    const highlights = pickAttributionSectorHighlights(SAMPLE_SECTORS, 'ASHARE', 1)

    assert.equal(highlights.positive.length, 1)
    assert.equal(highlights.positive[0].sector_name, '新能源')
    assert.equal(highlights.negative.length, 1)
    assert.equal(highlights.negative[0].sector_name, '银行')
  })
})
