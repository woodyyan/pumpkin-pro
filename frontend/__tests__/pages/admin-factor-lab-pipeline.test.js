import { describe, it } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const pageSource = readFileSync(new URL('../../pages/admin/data.js', import.meta.url), 'utf8')
const sectionsSource = readFileSync(new URL('../../components/admin/AdminSections.js', import.meta.url), 'utf8')

describe('admin factor lab pipeline labels', () => {
  it('updates operating cash flow repair labels to FCFM wording', () => {
    assert.match(pageSource, /AdminDataPage/)
    assert.match(sectionsSource, /repair_missing_fcfm_inputs/)
    assert.match(sectionsSource, /修复自由现金流率/)
    assert.match(sectionsSource, /自由现金流率 \(FCFM\)/)
    assert.doesNotMatch(sectionsSource, /经营现金流率/)
    assert.doesNotMatch(sectionsSource, /修复经营现金流/)
  })

  it('exposes industries phase0 mode and manual trigger', () => {
    assert.match(sectionsSource, /<option value="industries">industries<\/option>/)
    assert.match(sectionsSource, /只刷新行业/)
    assert.match(sectionsSource, /phase0_mode: 'industries'/)
  })

  it('shows dividends manual full refresh and industries health guidance', () => {
    assert.match(sectionsSource, /full_refresh_dividends/)
    assert.match(sectionsSource, /全量刷新股息率/)
    assert.match(sectionsSource, /默认不跑 dividends/)
    assert.match(sectionsSource, /行业刷新健康度/)
    assert.match(sectionsSource, /自动链路允许 warning 放行，不再 hard fail/)
  })

  it('shows data source health panel on admin data page', () => {
    assert.match(sectionsSource, /数据源健康/)
    assert.match(sectionsSource, /adminFetch\('\/api\/admin\/data-source-health'\)/)
    assert.match(sectionsSource, /adminFetch\('\/api\/admin\/company-profiles\/refresh'/)
    assert.match(sectionsSource, /Gateway 最近状态/)
    assert.match(sectionsSource, /公司资料同步/)
  })
})
