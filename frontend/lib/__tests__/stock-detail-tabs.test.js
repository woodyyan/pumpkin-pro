import { describe, it } from 'node:test'
import assert from 'node:assert/strict'
import {
  STOCK_DETAIL_TAB_KEYS,
  STOCK_DETAIL_TABS,
  getStockDetailMobileGroups,
  isStockDetailTabInMobileGroup,
  normalizeStockDetailTab,
} from '../stock-detail-tabs.js'

describe('stock-detail-tabs', () => {
  it('normalizes unknown, empty and array tab values to overview', () => {
    assert.equal(normalizeStockDetailTab(undefined), STOCK_DETAIL_TAB_KEYS.OVERVIEW)
    assert.equal(normalizeStockDetailTab(''), STOCK_DETAIL_TAB_KEYS.OVERVIEW)
    assert.equal(normalizeStockDetailTab('missing'), STOCK_DETAIL_TAB_KEYS.OVERVIEW)
    assert.equal(normalizeStockDetailTab(['technical']), STOCK_DETAIL_TAB_KEYS.TECHNICAL)
  })

  it('keeps the PC information architecture in the confirmed order', () => {
    assert.deepEqual(
      STOCK_DETAIL_TABS.map((tab) => tab.key),
      ['overview', 'chart', 'technical', 'fundamental', 'news', 'portfolio']
    )
  })

  it('groups mobile tabs into overview, chart, analysis, news and portfolio', () => {
    assert.deepEqual(
      getStockDetailMobileGroups().map((group) => group.key),
      ['overview', 'chart', 'analysis', 'news', 'portfolio']
    )
    assert.equal(isStockDetailTabInMobileGroup('technical', 'analysis'), true)
    assert.equal(isStockDetailTabInMobileGroup('fundamental', 'analysis'), true)
    assert.equal(isStockDetailTabInMobileGroup('news', 'news'), true)
    assert.equal(isStockDetailTabInMobileGroup('news', 'analysis'), false)
    assert.equal(isStockDetailTabInMobileGroup('portfolio', 'analysis'), false)
  })
})
