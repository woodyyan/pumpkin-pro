import { describe, it } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const pageSource = readFileSync(new URL('../../pages/live-trading.js', import.meta.url), 'utf8')

describe('live trading overview layout', () => {
  it('keeps the core index cards section', () => {
    assert.match(pageSource, /核心指数卡片/)
  })

  it('removes the focus chart module', () => {
    assert.doesNotMatch(pageSource, /主图查看/)
    assert.doesNotMatch(pageSource, /Focus Chart/)
    assert.doesNotMatch(pageSource, /function FocusIndexPanel\(/)
  })

  it('removes the secondary indexes module', () => {
    assert.doesNotMatch(pageSource, /扩展指数观察/)
    assert.doesNotMatch(pageSource, /Style Radar/)
    assert.doesNotMatch(pageSource, /function CompactIndexCard\(/)
  })

  it('keeps index cards as static cards without active selection state', () => {
    assert.match(pageSource, /function MarketIndexCard\(\{ index \}\)/)
    assert.doesNotMatch(pageSource, /activeIndexCode/)
    assert.doesNotMatch(pageSource, /onActivate/)
  })
})
