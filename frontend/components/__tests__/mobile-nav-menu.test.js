import { describe, it } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

import { buildNavigationState } from '../../lib/navigation.js'

const mobileNavSource = readFileSync(new URL('../MobileNavMenu.js', import.meta.url), 'utf8')
const appSource = readFileSync(new URL('../../pages/_app.js', import.meta.url), 'utf8')

describe('mobile nav grouped menu', () => {
  it('uses the new first-level group order and keeps tracking children together', () => {
    const state = buildNavigationState('/watchlist', 0)

    assert.deepEqual(state.groups.map((group) => group.label), ['卧龙AI', '看板', '跟踪', '选股', '更多'])
    assert.deepEqual(
      state.groups.find((group) => group.key === 'tracking').items.map((item) => item.label),
      ['自选股', '组合跟踪', '持仓管理']
    )
    assert.equal(state.activeGroupKey, 'tracking')
  })

  it('treats live-trading detail pages as the market entry under 看板', () => {
    const state = buildNavigationState('/live-trading/00700', 0)
    const dashboardGroup = state.groups.find((group) => group.key === 'dashboard')
    const marketItem = dashboardGroup.items.find((item) => item.key === 'market-overview')

    assert.equal(dashboardGroup.isActive, true)
    assert.equal(marketItem.isActive, true)
    assert.equal(state.activeGroupKey, 'dashboard')
  })

  it('renders the mobile menu as an independent fixed panel with its own overlay and scroll area', () => {
    assert.match(mobileNavSource, /className="fixed inset-x-0 bottom-0 top-16 z-40 md:hidden"/)
    assert.match(mobileNavSource, /aria-label="关闭移动导航菜单"/)
    assert.match(mobileNavSource, /role="dialog"/)
    assert.match(mobileNavSource, /overflow-y-auto/)
    assert.match(mobileNavSource, /onClick=\{onClose\}/)
    assert.doesNotMatch(mobileNavSource, /backdrop-blur-md/)
  })

  it('locks page scroll in _app and closes the menu through a dedicated callback', () => {
    assert.match(appSource, /document\.body\.style\.overflow = 'hidden'/)
    assert.match(appSource, /document\.documentElement\.style\.overflow = 'hidden'/)
    assert.match(appSource, /<MobileNavMenu open=\{mobileMenuOpen\} currentPath=\{currentPath\} unreadCount=\{unreadCount\} onClose=\{\(\) => setMobileMenuOpen\(false\)\} \/>/)
    assert.doesNotMatch(appSource, /mobileMenuRef/)
  })
})
