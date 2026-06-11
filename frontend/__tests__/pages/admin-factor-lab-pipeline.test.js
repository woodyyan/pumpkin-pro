import { describe, it } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const pageSource = readFileSync(new URL('../../pages/admin.js', import.meta.url), 'utf8')

describe('admin factor lab pipeline labels', () => {
  it('updates operating cash flow repair labels to FCFM wording', () => {
    assert.match(pageSource, /repair_missing_fcfm_inputs/)
    assert.match(pageSource, /修复自由现金流率/)
    assert.match(pageSource, /自由现金流率 \(FCFM\)/)
    assert.doesNotMatch(pageSource, /经营现金流率/)
    assert.doesNotMatch(pageSource, /修复经营现金流/)
  })

  it('exposes industries phase0 mode and manual trigger', () => {
    assert.match(pageSource, /<option value="industries">industries<\/option>/)
    assert.match(pageSource, /只刷新行业/)
    assert.match(pageSource, /phase0_mode: 'industries'/)
  })

  it('shows dividends manual full refresh and industries health guidance', () => {
    assert.match(pageSource, /full_refresh_dividends/)
    assert.match(pageSource, /全量刷新股息率/)
    assert.match(pageSource, /默认不跑 dividends/)
    assert.match(pageSource, /行业刷新健康度/)
    assert.match(pageSource, /自动链路允许 warning 放行，不再 hard fail/)
  })
})
