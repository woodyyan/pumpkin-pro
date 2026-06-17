import { describe, it } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const shellSource = readFileSync(new URL('../../components/admin/AdminShell.js', import.meta.url), 'utf8')
const navSource = readFileSync(new URL('../../components/admin/navigation.js', import.meta.url), 'utf8')
const appSource = readFileSync(new URL('../../pages/_app.js', import.meta.url), 'utf8')
const overviewRoute = readFileSync(new URL('../../pages/admin.js', import.meta.url), 'utf8')
const dataRoute = readFileSync(new URL('../../pages/admin/data.js', import.meta.url), 'utf8')
const aiRoute = readFileSync(new URL('../../pages/admin/ai.js', import.meta.url), 'utf8')
const opsRoute = readFileSync(new URL('../../pages/admin/ops.js', import.meta.url), 'utf8')

describe('admin navigation architecture', () => {
  it('registers sidebar sections and legacy tab redirects', () => {
    assert.match(navSource, /href: '\/admin'/)
    assert.match(navSource, /href: '\/admin\/data'/)
    assert.match(navSource, /href: '\/admin\/ai'/)
    assert.match(navSource, /href: '\/admin\/ops'/)
    assert.match(navSource, /payments: '\/admin\/ops'/)
    assert.match(navSource, /factor: '\/admin\/data'/)
    assert.match(navSource, /ai: '\/admin\/ai'/)
  })

  it('uses a shared shell with mobile drawer and logout/session bootstrap', () => {
    assert.match(shellSource, /adminFetch\('\/api\/admin\/session'\)/)
    assert.match(shellSource, /adminFetch\('\/api\/admin\/logout', \{ method: 'POST' \}\)/)
    assert.match(shellSource, /setMobileNavOpen/)
    assert.match(shellSource, /ADMIN_NAV_ITEMS\.map/)
    assert.match(shellSource, /切换管理导航/)
    assert.match(shellSource, /router\.replace\(\{ pathname: target, query: nextQuery \}/)
  })

  it('keeps admin routes outside the public app layout for all nested admin pages', () => {
    assert.match(appSource, /router\.pathname\.startsWith\('\/admin'\)/)
  })

  it('wires overview, data, ai and ops routes through the shell', () => {
    assert.match(overviewRoute, /<AdminShell section="overview">/)
    assert.match(dataRoute, /<AdminShell section="data">/)
    assert.match(aiRoute, /<AdminShell section="ai">/)
    assert.match(opsRoute, /<AdminShell section="ops">/)
  })
})
