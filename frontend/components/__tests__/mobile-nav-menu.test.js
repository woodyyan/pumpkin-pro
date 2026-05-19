import { describe, it } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const appSource = readFileSync(new URL('../../pages/_app.js', import.meta.url), 'utf8')

function getMobileNavBlock() {
  const start = appSource.indexOf('const NAV_ITEMS = [')
  const end = appSource.indexOf(']\n\nconst DESKTOP_NAV_ITEMS', start)

  assert.notEqual(start, -1, 'NAV_ITEMS definition not found')
  assert.notEqual(end, -1, 'NAV_ITEMS closing bracket not found')

  return appSource.slice(start, end)
}

describe('mobile nav menu config', () => {
  it('removes the home label and keeps the requested order', () => {
    const navBlock = getMobileNavBlock()
    const labels = [...navBlock.matchAll(/label: '([^']+)'/g)].map((match) => match[1])

    assert.equal(navBlock.includes("label: '首页'"), false)
    assert.deepEqual(labels, ['行情看板', '选股器', '回测引擎', '策略库', '持仓管理', '因子实验室', '更新日志'])
  })

  it('keeps the changelog badge key on the last mobile nav entry', () => {
    const navBlock = getMobileNavBlock()

    assert.ok(navBlock.includes("{ href: '/changelog', label: '更新日志', badgeKey: 'changelog' }"))
  })
})
