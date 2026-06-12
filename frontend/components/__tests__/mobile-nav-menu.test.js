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

  it('renders the mobile menu as a left slide-in drawer layered above its overlay', () => {
    // 抽屉容器铺满全屏，移动端专用
    assert.match(mobileNavSource, /className="fixed inset-0 z-40 md:hidden"/)
    // 遮罩按钮在底层 z-0，可关闭
    assert.match(mobileNavSource, /aria-label="关闭移动导航菜单"[\s\S]*?absolute inset-0 z-0 bg-black\/50/)
    // 面板从左侧滑入，显式置于上层 z-10
    assert.match(mobileNavSource, /role="dialog"[\s\S]*?absolute inset-y-0 left-0 z-10/)
    assert.match(mobileNavSource, /-translate-x-full/)
    assert.match(mobileNavSource, /translate-x-0/)
    // 不再使用此前易踩坑的 backdrop-blur-md
    assert.doesNotMatch(mobileNavSource, /backdrop-blur-md/)
  })

  it('flattens every group and its items so each entry is reachable in one tap', () => {
    // 平铺：不再有「点击一级菜单标题展开/收起」的折叠逻辑
    assert.doesNotMatch(mobileNavSource, /setExpandedGroupKey/)
    assert.doesNotMatch(mobileNavSource, /aria-expanded/)
    // 子项链接点击后关闭抽屉
    assert.match(mobileNavSource, /href=\{item\.href\}[\s\S]*?onClick=\{onClose\}/)
    // 支持 Esc 关闭
    assert.match(mobileNavSource, /event\.key === 'Escape'/)
  })

  it('locks page scroll in _app and closes the menu through a dedicated callback', () => {
    assert.match(appSource, /document\.body\.style\.overflow = 'hidden'/)
    assert.match(appSource, /document\.documentElement\.style\.overflow = 'hidden'/)
    assert.match(appSource, /<MobileNavMenu open=\{mobileMenuOpen\} currentPath=\{currentPath\} unreadCount=\{unreadCount\} onClose=\{\(\) => setMobileMenuOpen\(false\)\} \/>/)
    assert.doesNotMatch(appSource, /mobileMenuRef/)
  })
})
