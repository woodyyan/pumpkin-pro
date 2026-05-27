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
})
