import { describe, it } from 'node:test'
import assert from 'node:assert/strict'

const DESKTOP_NAV_ITEMS = [
  { href: '/live-trading', label: '行情看板' },
  { href: '/stock-picker', label: '选股器' },
  { href: '/backtest', label: '回测引擎' },
  { href: '/strategies', label: '策略库' },
  { href: '/portfolio', label: '持仓管理' },
]

const DESKTOP_MORE_ITEMS = [
  { href: '/factor-lab', label: '因子实验室' },
  { href: '/changelog', label: '更新日志', badgeKey: 'changelog' },
]

function buildDesktopNavState(currentPath, unreadCount) {
  return {
    primary: DESKTOP_NAV_ITEMS.map((item) => ({
      ...item,
      isActive: item.href === currentPath,
    })),
    more: {
      isActive: DESKTOP_MORE_ITEMS.some((item) => item.href === currentPath),
      badge: unreadCount > 0 ? (unreadCount > 99 ? '99+' : String(unreadCount)) : null,
      items: DESKTOP_MORE_ITEMS.map((item) => ({
        ...item,
        isActive: item.href === currentPath,
        badge: item.badgeKey === 'changelog' && unreadCount > 0 ? (unreadCount > 99 ? '99+' : String(unreadCount)) : null,
      })),
    },
  }
}

describe('desktop nav more menu', () => {
  it('keeps only five primary items in desktop nav', () => {
    const state = buildDesktopNavState('/live-trading', 0)
    assert.deepEqual(state.primary.map((item) => item.label), ['行情看板', '选股器', '回测引擎', '策略库', '持仓管理'])
  })

  it('marks more trigger active when route belongs to overflow items', () => {
    const factorLabState = buildDesktopNavState('/factor-lab', 0)
    const changelogState = buildDesktopNavState('/changelog', 3)

    assert.equal(factorLabState.more.isActive, true)
    assert.equal(changelogState.more.isActive, true)
    assert.equal(changelogState.primary.some((item) => item.isActive), false)
  })

  it('shows changelog unread badge on both trigger and menu item', () => {
    const state = buildDesktopNavState('/live-trading', 12)
    const changelogItem = state.more.items.find((item) => item.href === '/changelog')

    assert.equal(state.more.badge, '12')
    assert.equal(changelogItem.badge, '12')
  })

  it('caps large unread count at 99 plus', () => {
    const state = buildDesktopNavState('/live-trading', 120)
    const changelogItem = state.more.items.find((item) => item.href === '/changelog')

    assert.equal(state.more.badge, '99+')
    assert.equal(changelogItem.badge, '99+')
  })

  it('does not show badges when unread count is zero', () => {
    const state = buildDesktopNavState('/live-trading', 0)
    const changelogItem = state.more.items.find((item) => item.href === '/changelog')

    assert.equal(state.more.badge, null)
    assert.equal(changelogItem.badge, null)
  })
})
