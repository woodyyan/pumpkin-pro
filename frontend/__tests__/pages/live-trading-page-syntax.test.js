import { describe, it } from 'node:test'
import assert from 'node:assert/strict'
import { createRequire } from 'node:module'
import { readFileSync } from 'node:fs'

const requireFromCwd = createRequire(process.cwd() + '/')
const { parse } = requireFromCwd('next/dist/compiled/babel/parser')
const pageModule = requireFromCwd('./lib/live-trading-market.js')
const pageSource = readFileSync(new URL('../../pages/live-trading.js', import.meta.url), 'utf8')
const { buildMarketState, normalizeIndex, buildTrendSeries, formatMarketIndexTitle } = pageModule

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

  it('renders card-based market overview sections', () => {
    assert.ok(pageSource.includes('核心指数卡片'))
    assert.ok(pageSource.includes('扩展指数观察'))
    assert.ok(pageSource.includes('市场观察摘要'))
    assert.ok(pageSource.includes('MiniChart'))
  })

  it('maps core and secondary market indexes into cards', () => {
    const state = buildMarketState(
      {
        indexes: [
          { code: '000001', name: '上证指数', last: 3398.12, change_rate: 0.003, change_amount: 10.23 },
          { code: '399001', name: '深证成指', last: 10223.88, change_rate: -0.002, change_amount: -20.11 },
          { code: '399006', name: '创业板指', last: 2100.55, change_rate: 0.01, change_amount: 18.6 },
          { code: '000300', name: '沪深300', last: 4022.18, change_rate: 0.004, change_amount: 15.02 },
          { code: '000688', name: '科创50', last: 901.11, change_rate: 0.006, change_amount: 5.2 },
        ],
      },
      {
        indexes: [
          { code: 'HSI', name: 'Hang Seng Index', last: 18234.45, change_rate: 0.008, change_amount: 120.44 },
          { code: 'HSCEI', name: 'Hang Seng China Enterprises Index', last: 6455.9, change_rate: 0.005, change_amount: 30.18 },
          { code: 'HSTECH', name: 'Hang Seng TECH Index', last: 3801.22, change_rate: 0.012, change_amount: 42.98 },
        ],
      },
    )

    assert.equal(state.coreIndexes.length, 6)
    assert.equal(state.secondaryIndexes.length, 2)
    assert.equal(state.heroStats[0].value, '8 个')
    assert.equal(state.coreIndexes[0].title, '上证指数')
    assert.equal(state.secondaryIndexes[0].title, '沪深300')
    assert.ok(state.insights.some((item) => item.title === 'A/H 主市场对比'))
  })

  it('normalizes extra indexes and synthesizes trend points', () => {
    const item = normalizeIndex({ code: '000016', name: '上证50', last: 2500.11, change_rate: 0.002, change_amount: 5.1 })
    assert.equal(item.title, '上证50')
    assert.equal(item.market, 'A股')
    assert.equal(item.trend.length, 7)
    assert.equal(item.trend.at(-1).count, 2500.11)
  })

  it('keeps market title mapping for newly added indexes', () => {
    assert.equal(formatMarketIndexTitle('', '000300'), '沪深300')
    assert.equal(formatMarketIndexTitle('', '000688'), '科创50')
    assert.equal(formatMarketIndexTitle('', '000016'), '上证50')
    assert.equal(formatMarketIndexTitle('', '399905'), '中证500')
  })

  it('builds seven-point trend placeholders when upstream lacks chart data', () => {
    const trend = buildTrendSeries(100, 0.05)
    assert.equal(trend.length, 7)
    assert.equal(trend[0].date, '2026-06-01')
    assert.equal(trend.at(-1).count, 100)
  })
})
