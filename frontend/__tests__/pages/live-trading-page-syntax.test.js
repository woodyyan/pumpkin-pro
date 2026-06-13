import { describe, it } from 'node:test'
import assert from 'node:assert/strict'
import { createRequire } from 'node:module'
import { readFileSync } from 'node:fs'

const requireFromCwd = createRequire(process.cwd() + '/')
const { parse } = requireFromCwd('next/dist/compiled/babel/parser')
const pageSource = readFileSync(new URL('../../pages/live-trading.js', import.meta.url), 'utf8')

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
})
