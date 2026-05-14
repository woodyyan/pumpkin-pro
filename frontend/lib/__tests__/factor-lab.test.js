import { describe, it } from 'node:test'
import assert from 'node:assert/strict'

import {
  buildFactorScreenerPayload,
  buildSelectedFilterChips,
  codeToSymbol,
  flattenMetricDefinitions,
  formatFactorValue,
  getDynamicMetricColumns,
  getPageNumbers,
  updateFactorFilter,
} from '../factor-lab.js'

const groups = [
  { key: 'value', label: '价值', items: [{ key: 'pe', label: 'PE', unit: '倍', format: 'number' }] },
  { key: 'dividend', label: '股息率', items: [{ key: 'dividend_yield', label: '股息率', unit: '%', format: 'percentFromRatio' }] },
]

describe('buildFactorScreenerPayload', () => {
  it('drops empty filters and clamps pagination', () => {
    const payload = buildFactorScreenerPayload({
      filters: { pe: { min: '0', max: '20' }, pb: { min: '', max: '' } },
      sortBy: 'pe', sortOrder: 'desc', page: 0, pageSize: 999,
    })
    assert.deepEqual(payload.filters, { pe: { min: 0, max: 20 } })
    assert.equal(payload.sort_by, 'pe')
    assert.equal(payload.sort_order, 'desc')
    assert.equal(payload.page, 1)
    assert.equal(payload.page_size, 200)
  })
})

describe('updateFactorFilter', () => {
  it('removes a filter when both bounds are empty', () => {
    const next = updateFactorFilter({ pe: { min: '0', max: '20' } }, 'pe', 'min', '')
    assert.deepEqual(next, { pe: { min: '', max: '20' } })
    assert.deepEqual(updateFactorFilter(next, 'pe', 'max', ''), {})
  })
})

describe('formatFactorValue', () => {
  it('formats percent ratio and big numbers', () => {
    assert.equal(formatFactorValue(0.035, 'percentFromRatio'), '3.50%')
    assert.equal(formatFactorValue(12300000000, 'bigNumber'), '123.00 亿')
    assert.equal(formatFactorValue(null, 'number'), '--')
  })
})

describe('metric helpers', () => {
  it('flattens definitions and builds chips/dynamic columns', () => {
    const map = flattenMetricDefinitions(groups)
    assert.equal(map.pe.groupLabel, '价值')
    const chips = buildSelectedFilterChips({ pe: { max: '20' }, dividend_yield: { min: '3' } }, map)
    assert.equal(chips[0].text, 'PE ≤ 20倍')
    assert.equal(chips[1].text, '股息率 ≥ 3%')
    assert.deepEqual(getDynamicMetricColumns({ pe: {}, dividend_yield: {} }, map), ['pe', 'dividend_yield'])
  })
})

describe('navigation helpers', () => {
  it('builds page numbers and symbols', () => {
    assert.deepEqual(getPageNumbers(5, 10), [1, '...', 4, 5, 6, '...', 10])
    assert.equal(codeToSymbol('600519'), '600519.SH')
    assert.equal(codeToSymbol('1'), '000001.SZ')
  })
})
