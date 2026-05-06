import { describe, it } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const pageSource = readFileSync(new URL('../../pages/admin.js', import.meta.url), 'utf8')

describe('admin company profiles panel integration', () => {
  it('renders company profile management panel in dashboard', () => {
    assert.match(pageSource, /CompanyProfilesAdminPanel/)
    assert.match(pageSource, /公司资料管理/)
    assert.match(pageSource, /一键更新静态资料/)
  })

  it('wires overview, refresh and status endpoints', () => {
    assert.match(pageSource, /\/api\/admin\/company-profiles'/)
    assert.match(pageSource, /\/api\/admin\/company-profiles\/refresh'/)
    assert.match(pageSource, /refresh\.running/)
    assert.match(pageSource, /刷新失败：\{refresh\.error\}/)
  })

  it('shows coverage and failure items', () => {
    assert.match(pageSource, /coverage_rate/)
    assert.match(pageSource, /失败项 \/ 待补全/)
    assert.match(pageSource, /quality_flags/)
  })
})
