import { describe, it } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const pageSource = readFileSync(new URL('../../pages/live-trading.js', import.meta.url), 'utf8')

describe('live trading overview layout', () => {
  it('keeps the factor index and core index sections while removing extra helper copy', () => {
    assert.match(pageSource, /单因子指数/)
    assert.match(pageSource, /核心指数卡片/)
    assert.doesNotMatch(pageSource, /展示后端返回的真实指数趋势序列/)
    assert.doesNotMatch(pageSource, /行情时间/)
    assert.doesNotMatch(pageSource, /这里专注展示大盘指数/)
    assert.doesNotMatch(pageSource, /首屏保留 A 股与港股各自最重要的宽基与科技主线/)
    assert.doesNotMatch(pageSource, /首屏核心 \+ 扩展风格指数一并观察/)
    assert.doesNotMatch(pageSource, /用于快速判断市场广度/)
    assert.doesNotMatch(pageSource, /用少量文字解释今天的指数强弱分布/)
    assert.doesNotMatch(pageSource, /真实趋势/)
    assert.doesNotMatch(pageSource, / 点/)
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

  it('keeps factor and market cards as static cards without active selection state', () => {
    assert.match(pageSource, /function FactorIndexCard\(\{ item \}\)/)
    assert.match(pageSource, /function MarketIndexCard\(\{ index \}\)/)
    assert.doesNotMatch(pageSource, /activeIndexCode/)
    assert.doesNotMatch(pageSource, /onActivate/)
  })
})
