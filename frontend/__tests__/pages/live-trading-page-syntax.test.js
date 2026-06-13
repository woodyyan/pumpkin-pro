import { describe, it } from 'node:test'
import assert from 'node:assert/strict'
import { createRequire } from 'node:module'
import { readFileSync } from 'node:fs'

const requireFromCwd = createRequire(process.cwd() + '/')
const { parse } = requireFromCwd('next/dist/compiled/babel/parser')
const pageModule = requireFromCwd('./lib/live-trading-market.js')
const pageSource = readFileSync(new URL('../../pages/live-trading.js', import.meta.url), 'utf8')
const {
  buildMarketState,
  inferExchange,
  mapTrendPoints,
  normalizeIndex,
  normalizeTrendSeries,
  formatMarketIndexTitle,
} = pageModule

describe('live-trading page syntax', () => {
  it('parses as valid JSX', () => {
    assert.doesNotThrow(() => {
      parse(pageSource, {
        sourceType: 'module',
        plugins: ['jsx'],
      })
    })
  })

  it('keeps only market overview data on the page', () => {
    assert.ok(pageSource.includes("requestJson('/api/live/market/overview?exchange=SSE')"))
    assert.ok(pageSource.includes("requestJson('/api/live/market/overview')"))
    assert.ok(!pageSource.includes('/api/live/watchlist'))
    assert.ok(!pageSource.includes('添加关注股票'))
    assert.ok(!pageSource.includes('signalConfigMap'))
  })

  it('keeps 10-second polling for market indexes', () => {
    assert.ok(pageSource.includes('window.setInterval(() => {'))
    assert.ok(pageSource.includes('loadMarketOverview()'))
    assert.ok(pageSource.includes('10000'))
  })

  it('points users to watchlist for stock-level actions', () => {
    assert.ok(pageSource.includes('Link href="/watchlist"'))
    assert.ok(pageSource.includes('个股关注与实时卡片已迁移到自选股页面'))
    assert.ok(pageSource.includes('canonical" href="https://wolongtrader.top/live-trading"'))
  })

  it('renders second-stage focus chart interactions', () => {
    assert.ok(pageSource.includes('主图查看'))
    assert.ok(pageSource.includes('FocusIndexPanel'))
    assert.ok(pageSource.includes('onClick={() => onActivate(index.code)}'))
    assert.ok(pageSource.includes('真实趋势'))
    assert.ok(!pageSource.includes('占位趋势'))
  })

  it('maps core and secondary market indexes into cards', () => {
    const state = buildMarketState(
      {
        ts: '2026-06-13T14:30:00Z',
        trend_summary: 'A股震荡偏强',
        indexes: [
          {
            code: '000001',
            name: '上证指数',
            last: 3398.12,
            change_rate: 0.003,
            change_amount: 10.23,
            trend_points: [['2026-06-10', 3300], ['2026-06-11', 3340], ['2026-06-12', 3398.12]],
          },
          {
            code: '399001',
            name: '深证成指',
            last: 10223.88,
            change_rate: -0.002,
            change_amount: -20.11,
            trend_points: [['2026-06-10', 10280], ['2026-06-11', 10260], ['2026-06-12', 10223.88]],
          },
          {
            code: '399006',
            name: '创业板指',
            last: 2100.55,
            change_rate: 0.01,
            change_amount: 18.6,
            trend_points: [['2026-06-10', 2060], ['2026-06-11', 2080], ['2026-06-12', 2100.55]],
          },
          {
            code: '000300',
            name: '沪深300',
            last: 4022.18,
            change_rate: 0.004,
            change_amount: 15.02,
            trend_points: [['2026-06-10', 3980], ['2026-06-11', 4002], ['2026-06-12', 4022.18]],
          },
          {
            code: '000688',
            name: '科创50',
            last: 901.11,
            change_rate: 0.006,
            change_amount: 5.2,
            trend_points: [['2026-06-10', 888], ['2026-06-11', 894], ['2026-06-12', 901.11]],
          },
        ],
      },
      {
        ts: '2026-06-13T14:31:00Z',
        trend_summary: '港股科技更强',
        indexes: [
          {
            code: 'HSI',
            name: 'Hang Seng Index',
            last: 18234.45,
            change_rate: 0.008,
            change_amount: 120.44,
            trend_points: [['2026-06-10', 17980], ['2026-06-11', 18090], ['2026-06-12', 18234.45]],
          },
          {
            code: 'HSCEI',
            name: 'Hang Seng China Enterprises Index',
            last: 6455.9,
            change_rate: 0.005,
            change_amount: 30.18,
            trend_points: [['2026-06-10', 6380], ['2026-06-11', 6420], ['2026-06-12', 6455.9]],
          },
          {
            code: 'HSTECH',
            name: 'Hang Seng TECH Index',
            last: 3801.22,
            change_rate: 0.012,
            change_amount: 42.98,
            trend_points: [['2026-06-10', 3700], ['2026-06-11', 3750], ['2026-06-12', 3801.22]],
          },
        ],
      },
    )

    assert.equal(state.coreIndexes.length, 6)
    assert.equal(state.secondaryIndexes.length, 2)
    assert.equal(state.heroStats[0].value, '8 个')
    assert.equal(state.coreIndexes[0].title, '上证指数')
    assert.equal(state.secondaryIndexes[0].title, '沪深300')
    assert.equal(state.updatedAt, '2026-06-13T14:31:00Z')
    assert.equal(state.trendSummary, 'A股震荡偏强；港股科技更强')
    assert.ok(state.insights.some((item) => item.title === 'A/H 主市场对比'))
  })

  it('prefers real trend points when upstream provides them', () => {
    const item = normalizeIndex({
      code: '000016',
      name: '上证50',
      last: 2500.11,
      change_rate: 0.002,
      change_amount: 5.1,
      trend_points: [
        ['2026-06-10', 2480.1],
        ['2026-06-11', 2490.2],
        ['2026-06-12', 2500.11],
      ],
    })

    assert.equal(item.title, '上证50')
    assert.equal(item.market, 'A股')
    assert.equal(item.trend.length, 3)
    assert.equal(item.chartMeta.hasRealTrend, true)
    assert.equal(item.trend.at(-1).count, 2500.11)
  })

  it('returns empty trend when upstream does not provide real series', () => {
    const trend = normalizeTrendSeries({ code: '000001', name: '上证指数' })

    assert.equal(trend.length, 0)
  })

  it('maps generic trend point shapes', () => {
    const trend = mapTrendPoints([
      { date: '2026-06-10', value: 10 },
      ['2026-06-11', 12],
      14,
    ])

    assert.equal(trend.length, 3)
    assert.equal(trend[0].count, 10)
    assert.equal(trend[1].count, 12)
    assert.equal(trend[2].count, 14)
  })

  it('infers exchange from index code', () => {
    assert.equal(inferExchange({ code: 'HSI' }), 'HKEX')
    assert.equal(inferExchange({ code: '399001' }), 'SSE')
  })

  it('keeps market title mapping for newly added indexes', () => {
    assert.equal(formatMarketIndexTitle('', '000300'), '沪深300')
    assert.equal(formatMarketIndexTitle('', '000688'), '科创50')
    assert.equal(formatMarketIndexTitle('', '000016'), '上证50')
    assert.equal(formatMarketIndexTitle('', '399905'), '中证500')
  })

  it('drops indexes without real trend points', () => {
    const item = normalizeIndex({
      code: '000001',
      name: '上证指数',
      last: 3398.12,
      change_rate: 0.003,
    })

    assert.equal(item, null)
  })
})
