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

  it('uses unified trade-date helper for quadrant data labels', () => {
    assert.ok(pageSource.includes("formatCloseDateLabel(quadrantData.meta.source_trade_date, quadrantData.meta.computed_at)"))
    assert.ok(pageSource.includes('parseTradeDateLabelDate(quadrantData.meta.source_trade_date)'))
    assert.ok(!pageSource.includes('数据日期：{formatDateTime(quadrantData.meta.computed_at)}'))
  })

  it('keeps only market and watchlist on 10-second polling', () => {
    assert.ok(!pageSource.includes('行情看板概览'))
    assert.ok(!pageSource.includes('手动刷新'))
    assert.ok(!pageSource.includes('manualRefreshing'))
    assert.ok(!pageSource.includes('handleManualRefresh'))
    assert.ok(pageSource.includes('const refreshRealtimeSections = useCallback(() => {'))
    assert.ok(pageSource.includes('loadPublicData()'))
    assert.ok(pageSource.includes('loadPrivateData()'))
    assert.ok(pageSource.includes('window.setInterval(() => {'))
    assert.ok(pageSource.includes('refreshRealtimeSections()'))
    assert.ok(pageSource.includes('10000'))

    const refreshStart = pageSource.indexOf('const refreshRealtimeSections = useCallback(() => {')
    assert.notEqual(refreshStart, -1, 'refreshRealtimeSections definition not found')

    const refreshEnd = pageSource.indexOf('}, [privateAccessReady])', refreshStart)
    assert.notEqual(refreshEnd, -1, 'refreshRealtimeSections closing not found')

    const refreshBody = pageSource.slice(refreshStart, refreshEnd)
    assert.ok(refreshBody.includes('loadPublicData()'))
    assert.ok(refreshBody.includes('loadPrivateData()'))
    assert.ok(!refreshBody.includes('loadRanking('))
    assert.ok(!refreshBody.includes('loadRankingPortfolio('))
  })
})
