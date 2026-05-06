import { describe, it } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const source = readFileSync(new URL('../CompanyAboutPanel.js', import.meta.url), 'utf8')

describe('CompanyAboutPanel source contract', () => {
  it('renders full listed profile fields and safe website link', () => {
    assert.match(source, /关于这家公司/)
    assert.match(source, /一句话业务介绍/)
    assert.match(source, /profile\?\.business_summary/)
    assert.match(source, /profile\?\.industry_name/)
    assert.match(source, /target="_blank"/)
    assert.match(source, /rel="noreferrer"/)
  })

  it('renders pending, loading, error and delisted states', () => {
    assert.match(source, /公司资料加载中/)
    assert.match(source, /资料整理中，暂未收录该公司的静态资料/)
    assert.match(source, /error \? \(/)
    assert.match(source, /资料保留为最后一次收录信息/)
    assert.match(source, /formatListingStatus\(status\)/)
  })

  it('keeps raw business scope behind details expansion', () => {
    assert.match(source, /<details/)
    assert.match(source, /查看原始业务资料/)
    assert.match(source, /profile\.business_scope/)
  })
})
