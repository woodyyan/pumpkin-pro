import { describe, it } from 'node:test'
import assert from 'node:assert/strict'
import { createRequire } from 'node:module'
import { readFileSync } from 'node:fs'

const requireFromCwd = createRequire(process.cwd() + '/')
const { parse } = requireFromCwd('next/dist/compiled/babel/parser')
const pageSource = readFileSync(new URL('../../pages/changelog.js', import.meta.url), 'utf8')
const changelogSource = readFileSync(new URL('../../data/changelog.json', import.meta.url), 'utf8')
const changelogData = JSON.parse(changelogSource)

describe('changelog page', () => {
  it('parses as valid JSX', () => {
    assert.doesNotThrow(() => parse(pageSource, { sourceType: 'module', plugins: ['jsx'] }))
  })

  it('shows the 2.0 upgrade entry as the latest visible log', () => {
    assert.equal(changelogData.last_updated, '2026-06-12')
    assert.equal(changelogData.items[0].date, '2026-06-12')
    assert.equal(changelogData.items[0].title, '卧龙AI量化交易台升级 2.0，选股更专业，使用更简单')
    assert.match(changelogData.items[0].summary, /更多实用的 AI 能力/)
    assert.match(changelogData.items[0].summary, /量化因子选股/)
    assert.match(changelogData.items[0].summary, /模拟组合/)
    assert.match(changelogData.items[0].summary, /敬请期待/)
  })

  it('continues to render changelog entries from the shared json data source', () => {
    assert.match(pageSource, /import changelogData from '..\/data\/changelog.json'/)
    assert.match(pageSource, /filter\(\(item\) => item\?\.visible !== false\)/)
    assert.match(pageSource, /sort\(\(left, right\) => String\(right.date \|\| ''\)\.localeCompare\(String\(left.date \|\| ''\)\)\)/)
  })
})
