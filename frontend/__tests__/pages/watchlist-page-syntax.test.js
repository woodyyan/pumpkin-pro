import { describe, it } from 'node:test'
import assert from 'node:assert/strict'
import { createRequire } from 'node:module'
import { readFileSync } from 'node:fs'

const requireFromCwd = createRequire(process.cwd() + '/')
const { parse } = requireFromCwd('next/dist/compiled/babel/parser')
const pageSource = readFileSync(new URL('../../pages/watchlist.js', import.meta.url), 'utf8')

describe('watchlist page syntax', () => {
  it('parses as valid JSX', () => {
    assert.doesNotThrow(() => {
      parse(pageSource, {
        sourceType: 'module',
        plugins: ['jsx'],
      })
    })
  })

  it('loads only private watchlist data and does not fetch market overview', () => {
    assert.ok(pageSource.includes("requestJson('/api/live/watchlist')"))
    assert.ok(pageSource.includes("requestJson('/api/live/watchlist/snapshots')"))
    assert.ok(pageSource.includes("requestJson('/api/signal-configs')"))
    assert.ok(!pageSource.includes('/api/live/market/overview'))
  })

  it('uses auth gating and keeps 10-second polling for watchlist cards', () => {
    assert.ok(pageSource.includes('const privateAccessReady = ready && isLoggedIn'))
    assert.ok(pageSource.includes('if (privateAccessReady) {'))
    assert.ok(pageSource.includes('if (!privateAccessReady) return undefined'))
    assert.ok(pageSource.includes('window.setInterval(() => {'))
    assert.ok(pageSource.includes('loadPrivateData()'))
    assert.ok(pageSource.includes('10000'))
  })

  it('keeps the add form and guest shell on watchlist', () => {
    assert.ok(pageSource.includes('添加关注股票'))
    assert.ok(pageSource.includes('登录后开启自选股'))
    assert.ok(pageSource.includes('canonical" href="https://wolongtrader.top/watchlist"'))
  })
})
