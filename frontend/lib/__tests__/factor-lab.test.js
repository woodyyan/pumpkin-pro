import { describe, it } from 'node:test'
import assert from 'node:assert/strict'

import {
  areFactorWeightsEqual,
  buildFactorScreenerPayload,
  codeToSymbol,
  factorWeightChipText,
  flattenFactorDefinitions,
  formatFactorValue,
  formatWeight,
  getActiveFactorScoreKeys,
  getPageNumbers,
  isScoreColumnActive,
  normalizeFactorWeights,
  sumFactorWeights,
  validateFactorWeights,
} from '../factor-lab.js'

describe('buildFactorScreenerPayload', () => {
  it('normalizes positive factor weights and clamps pagination', () => {
    const payload = buildFactorScreenerPayload({
      factorWeights: { value: '12.5%', growth: '87.5', quality: '', momentum: '0' },
      sortBy: 'growth_score', sortOrder: 'asc', page: 0, pageSize: 999,
    })
    assert.deepEqual(payload.factor_weights, { value: 12.5, growth: 87.5 })
    assert.equal(payload.sort_by, 'growth_score')
    assert.equal(payload.sort_order, 'asc')
    assert.equal(payload.page, 1)
    assert.equal(payload.page_size, 200)
  })
})

describe('factor weight helpers', () => {
  it('validates default equal-weight mode and custom weights', () => {
    assert.deepEqual(validateFactorWeights({}), { valid: true, sum: 100, message: '未选择因子时将恢复默认 7 因子等权结果。' })
    assert.equal(validateFactorWeights({ value: '50' }).valid, false)
    assert.equal(validateFactorWeights({ value: '12.5', growth: '87.5' }).valid, true)
    assert.equal(validateFactorWeights({ value: '100.01' }).valid, false)
    assert.deepEqual(normalizeFactorWeights({ value: '12.5%', growth: '87.5', size: '-1', quality: '100.001' }), { value: 12.5, growth: 87.5 })
    assert.equal(sumFactorWeights({ value: '12.5', quality: '87.5' }), 100)
  })

  it('builds factor chips from definitions', () => {
    const map = flattenFactorDefinitions([{ key: 'value', scoreKey: 'value_score', label: '价值' }])
    assert.deepEqual(factorWeightChipText({}, map), ['默认 7 因子等权'])
    assert.deepEqual(factorWeightChipText({ value: '12.5' }, map), ['价值 12.5%'])
    assert.equal(formatWeight(30), '30%')
  })

  it('marks only weighted factors as active when custom weights exist', () => {
    const map = flattenFactorDefinitions([
      { key: 'value', scoreKey: 'value_score', label: '价值' },
      { key: 'growth', scoreKey: 'growth_score', label: '成长' },
      { key: 'quality', scoreKey: 'quality_score', label: '质量' },
    ])
    const active = getActiveFactorScoreKeys({ value: '50', growth: '50', quality: '' }, map)
    assert.equal(isScoreColumnActive('composite_score', active), true)
    assert.equal(isScoreColumnActive('value_score', active), true)
    assert.equal(isScoreColumnActive('growth_score', active), true)
    assert.equal(isScoreColumnActive('quality_score', active), false)
    assert.equal(isScoreColumnActive('close_price', active), true)
  })

  it('compares draft and applied weights by selected keys and normalized values', () => {
    assert.equal(areFactorWeightsEqual({}, {}), true)
    assert.equal(areFactorWeightsEqual({ value: '30' }, { value: '30.0%' }), true)
    assert.equal(areFactorWeightsEqual({ value: '' }, {}), false)
    assert.equal(areFactorWeightsEqual({ value: '30' }, { growth: '30' }), false)
  })

  it('keeps every factor score column active in default equal-weight mode', () => {
    const active = getActiveFactorScoreKeys({})
    assert.equal(isScoreColumnActive('value_score', active), true)
    assert.equal(isScoreColumnActive('low_volatility_score', active), true)
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
