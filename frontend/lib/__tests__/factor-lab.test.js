import { describe, it } from 'node:test'
import assert from 'node:assert/strict'

import {
  buildFactorScreenerPayload,
  codeToSymbol,
  factorWeightChipText,
  flattenFactorDefinitions,
  formatFactorValue,
  getPageNumbers,
  normalizeFactorWeights,
  sumFactorWeights,
  validateFactorWeights,
} from '../factor-lab.js'

describe('buildFactorScreenerPayload', () => {
  it('normalizes positive factor weights and clamps pagination', () => {
    const payload = buildFactorScreenerPayload({
      factorWeights: { value: '0.4', growth: '0.6', quality: '', momentum: '0' },
      sortBy: 'growth_score', sortOrder: 'asc', page: 0, pageSize: 999,
    })
    assert.deepEqual(payload.factor_weights, { value: 0.4, growth: 0.6 })
    assert.equal(payload.sort_by, 'growth_score')
    assert.equal(payload.sort_order, 'asc')
    assert.equal(payload.page, 1)
    assert.equal(payload.page_size, 200)
  })
})

describe('factor weight helpers', () => {
  it('validates default equal-weight mode and custom weights', () => {
    assert.equal(validateFactorWeights({}).valid, true)
    assert.equal(validateFactorWeights({ value: '0.5' }).valid, false)
    assert.equal(validateFactorWeights({ value: '0.5', growth: '0.5' }).valid, true)
    assert.deepEqual(normalizeFactorWeights({ value: '0.5', growth: '0.5', size: '-1' }), { value: 0.5, growth: 0.5 })
    assert.equal(sumFactorWeights({ value: '0.25', quality: '0.75' }), 1)
  })

  it('builds factor chips from definitions', () => {
    const map = flattenFactorDefinitions([{ key: 'value', scoreKey: 'value_score', label: '价值' }])
    assert.deepEqual(factorWeightChipText({}, map), ['默认 7 因子等权'])
    assert.deepEqual(factorWeightChipText({ value: '1' }, map), ['价值 1.00'])
  })
})

describe('formatFactorValue', () => {
  it('formats score, percent ratio and big numbers', () => {
    assert.equal(formatFactorValue(88.888, 'score'), '88.9')
    assert.equal(formatFactorValue(0.035, 'percentFromRatio'), '3.50%')
    assert.equal(formatFactorValue(12300000000, 'bigNumber'), '123.00 亿')
    assert.equal(formatFactorValue(null, 'number'), '--')
  })
})

describe('navigation helpers', () => {
  it('builds page numbers and symbols', () => {
    assert.deepEqual(getPageNumbers(5, 10), [1, '...', 4, 5, 6, '...', 10])
    assert.equal(codeToSymbol('600519'), '600519.SH')
    assert.equal(codeToSymbol('1'), '000001.SZ')
  })
})
