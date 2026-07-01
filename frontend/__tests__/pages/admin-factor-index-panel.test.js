import { describe, it } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const sectionsSource = readFileSync(new URL('../../components/admin/AdminSections.js', import.meta.url), 'utf8')
const panelSource = readFileSync(new URL('../../components/admin/FactorIndexAdminPanel.js', import.meta.url), 'utf8')

describe('admin factor index panel', () => {
  it('wires the panel into the admin data page and fetches admin factor index status', () => {
    assert.match(sectionsSource, /import FactorIndexAdminPanel from '\.\/FactorIndexAdminPanel'/)
    assert.match(sectionsSource, /<FactorIndexAdminPanel onUnauthorized=\{onUnauthorized\} \/>/)
    assert.match(panelSource, /adminFetch\('\/api\/admin\/factor-index\/status'\)/)
    assert.match(panelSource, /adminFetch\('\/api\/admin\/factor-index\/recompute'/)
  })

  it('provides scoped recompute controls and full rebuild copy', () => {
    assert.match(panelSource, /sync_all/)
    assert.match(panelSource, /sync_daily/)
    assert.match(panelSource, /sync_rebalances/)
    assert.match(panelSource, /type="date"/)
    assert.match(panelSource, /从头重建全部指数/)
    assert.match(panelSource, /reset/)
  })
})
