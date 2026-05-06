import { describe, it } from 'node:test'
import assert from 'node:assert/strict'

import {
  buildCompanyAboutPath,
  extractDisplayDomain,
  formatAboutDate,
  formatListingStatus,
  isSafeWebsiteUrl,
  normalizeWebsiteHref,
} from '../company-about.js'

describe('company about helpers', () => {
  it('builds encoded about API path', () => {
    assert.equal(buildCompanyAboutPath(' 00700.hk '), '/api/live/symbols/00700.HK/about')
    assert.equal(buildCompanyAboutPath(''), '')
  })

  it('maps listing status labels', () => {
    assert.equal(formatListingStatus('LISTED'), '已上市')
    assert.equal(formatListingStatus('DELISTED'), '已退市')
    assert.equal(formatListingStatus('bad'), '未确认')
  })

  it('formats dates by precision', () => {
    assert.equal(formatAboutDate('1999-11-20', 'day'), '1999-11-20')
    assert.equal(formatAboutDate('1999-11-20', 'month'), '1999-11')
    assert.equal(formatAboutDate('1999-11-20', 'year'), '1999')
    assert.equal(formatAboutDate('', 'day'), '--')
  })

  it('extracts display domain and normalizes safe website href', () => {
    assert.equal(extractDisplayDomain('https://www.moutaichina.com/about'), 'moutaichina.com')
    assert.equal(normalizeWebsiteHref('example.com'), 'https://example.com')
    assert.equal(isSafeWebsiteUrl('javascript:alert(1)'), false)
  })
})
