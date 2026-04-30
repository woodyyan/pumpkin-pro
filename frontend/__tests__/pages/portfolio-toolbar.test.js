import { describe, it } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const pageSource = readFileSync(new URL('../../pages/portfolio.js', import.meta.url), 'utf8')

describe('portfolio toolbar simplification', () => {
  it('keeps only the scope switcher in Toolbar', () => {
    const toolbarMatch = pageSource.match(/function Toolbar\([\s\S]*?^\}\n/m)
    if (!toolbarMatch) {
      assert.fail('Toolbar not found')
    }
    const toolbarBody = toolbarMatch[0]

    assert.match(toolbarBody, /SCOPE_OPTIONS\.map/)
    assert.doesNotMatch(toolbarBody, /搜索代码或名称/)
    assert.doesNotMatch(toolbarBody, /SORT_OPTIONS/)
    assert.doesNotMatch(toolbarBody, /PNL_FILTER_OPTIONS/)
    assert.doesNotMatch(toolbarBody, /切换为升序/)
    assert.doesNotMatch(toolbarBody, /切换为降序/)
  })

  it('stops wiring removed search and sort state into dashboard loading', () => {
    assert.doesNotMatch(pageSource, /const \[sortBy, setSortBy\] = useState/)
    assert.doesNotMatch(pageSource, /const \[sortOrder, setSortOrder\] = useState/)
    assert.doesNotMatch(pageSource, /const \[pnlFilter, setPnlFilter\] = useState/)
    assert.doesNotMatch(pageSource, /const \[keyword, setKeyword\] = useState/)
    assert.doesNotMatch(pageSource, /sort_by: sortBy/)
    assert.doesNotMatch(pageSource, /sort_order: sortOrder/)
    assert.doesNotMatch(pageSource, /pnl_filter: pnlFilter/)
    assert.doesNotMatch(pageSource, /keyword,/)
  })
})
