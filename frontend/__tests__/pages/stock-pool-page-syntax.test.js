import { describe, it } from 'node:test'
import assert from 'node:assert/strict'
import { createRequire } from 'node:module'
import { readFileSync } from 'node:fs'

const requireFromCwd = createRequire(process.cwd() + '/')
const { parse } = requireFromCwd('next/dist/compiled/babel/parser')
const pageSource = readFileSync(new URL('../../pages/stock-pool.js', import.meta.url), 'utf8')
const navSource = readFileSync(new URL('../../lib/navigation.js', import.meta.url), 'utf8')

describe('stock-pool page syntax', () => {
  it('parses as valid JSX', () => {
    assert.doesNotThrow(() => {
      parse(pageSource, { sourceType: 'module', plugins: ['jsx'] })
    })
  })

  it('loads the aggregated factor pool once per refresh interval', () => {
    assert.ok(pageSource.includes("requestJson('/api/live/factor-pool')"))
    assert.ok(pageSource.includes('window.setInterval(loadPool, 10000)'))
    assert.ok(!pageSource.includes('/api/live/symbols/'))
  })

  it('keeps the factor pool in the dashboard navigation', () => {
    assert.ok(navSource.includes("{ key: 'stock-pool', href: '/stock-pool', label: '卧龙股池'"))
  })

  it('renders both desktop lists and the mobile factor switcher', () => {
    assert.ok(pageSource.includes('md:grid-cols-2'))
    assert.ok(pageSource.includes("setActiveFactor(list.factorKey)"))
    assert.ok(pageSource.includes('因子分仅用于同一榜单内排序'))
  })
})
