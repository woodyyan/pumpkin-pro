import { describe, it } from 'node:test'
import assert from 'node:assert/strict'
import { createRequire } from 'node:module'
import { readFileSync } from 'node:fs'

const requireFromCwd = createRequire(process.cwd() + '/')
const { parse } = requireFromCwd('next/dist/compiled/babel/parser')
const pageSource = readFileSync(new URL('../../pages/capital-map.js', import.meta.url), 'utf8')
const dashboardSource = readFileSync(new URL('../../components/CapitalMapDashboard.js', import.meta.url), 'utf8')

describe('capital map page presentation', () => {
  it('parses the page and dashboard component', () => {
    assert.doesNotThrow(() => parse(pageSource, { sourceType: 'module', plugins: ['jsx'] }))
    assert.doesNotThrow(() => parse(dashboardSource, { sourceType: 'module', plugins: ['jsx'] }))
  })

  it('keeps source and refresh failures invisible in the UI', () => {
    assert.doesNotMatch(dashboardSource, /数据源：/)
    assert.doesNotMatch(dashboardSource, /手动刷新/)
    assert.doesNotMatch(dashboardSource, /重新获取数据/)
    assert.doesNotMatch(dashboardSource, /行情源刷新失败/)
  })

  it('uses the approved data disclaimer copy', () => {
    assert.match(dashboardSource, /当前按成交额排序抓取高流动性样本。主力净流入属于平台算法口径，不等同于交易所逐笔资金流。本页仅用于市场观察和产品验证，不构成投资建议。/)
  })
})
