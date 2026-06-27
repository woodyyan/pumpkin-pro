export const NAV_GROUPS = [
  {
    key: 'wolong-ai',
    label: '卧龙AI',
    items: [
      { key: 'ai-analysis', href: '/ai/analysis', label: 'AI分析', matchNested: true },
      { key: 'ai-reports', href: '/ai/reports', label: 'AI研报', matchNested: true },
      { key: 'ai-picker', href: '/ai/picker', label: 'AI选股', matchNested: true },
      { key: 'ai-backtest', href: '/ai/backtest', label: 'AI回测', matchNested: true },
    ],
  },
  {
    key: 'dashboard',
    label: '看板',
    items: [
      { key: 'market-overview', href: '/live-trading', label: '市场行情', matchNested: true },
      { key: 'quadrant', href: '/quadrant', label: '市场全景', matchNested: true },
    ],
  },
  {
    key: 'tracking',
    label: '跟踪',
    items: [
      { key: 'watchlist', href: '/watchlist', label: '自选股', matchNested: true },
      { key: 'portfolio-tracking', href: '/portfolio-tracking', label: '组合跟踪', matchNested: true },
      { key: 'portfolio', href: '/portfolio', label: '持仓管理', matchNested: true },
    ],
  },
  {
    key: 'screening',
    label: '选股',
    items: [
      { key: 'stock-picker', href: '/stock-picker', label: '选股器', matchNested: true },
      { key: 'factor-lab', href: '/factor-lab', label: '因子实验室', matchNested: true },
      { key: 'backtest', href: '/backtest', label: '回测引擎', matchNested: true },
      { key: 'strategies', href: '/strategies', label: '策略库', matchNested: true },
    ],
  },
  {
    key: 'more',
    label: '更多',
    items: [
      { key: 'changelog', href: '/changelog', label: '更新日志', badgeKey: 'changelog', matchNested: true },
    ],
  },
]

function normalizePath(path) {
  if (!path) return '/'

  const [pathname] = String(path).split(/[?#]/, 1)
  if (!pathname) return '/'
  if (pathname.length > 1 && pathname.endsWith('/')) {
    return pathname.slice(0, -1)
  }

  return pathname
}

export function formatNavBadgeCount(count) {
  if (!Number.isFinite(count) || count <= 0) return null
  return count > 99 ? '99+' : String(count)
}

export function isNavItemActive(currentPath, item) {
  const normalizedCurrentPath = normalizePath(currentPath)
  const normalizedHref = normalizePath(item.href)

  if (normalizedCurrentPath === normalizedHref) {
    return true
  }

  if (item.matchNested && normalizedCurrentPath.startsWith(`${normalizedHref}/`)) {
    return true
  }

  return false
}

export function buildNavigationState(currentPath, unreadCount = 0) {
  const groups = NAV_GROUPS.map((group) => {
    const items = group.items.map((item) => {
      const badge = item.badgeKey === 'changelog' ? formatNavBadgeCount(unreadCount) : null
      return {
        ...item,
        isActive: isNavItemActive(currentPath, item),
        badge,
      }
    })

    return {
      ...group,
      items,
      isActive: items.some((item) => item.isActive),
      badge: items.map((item) => item.badge).find(Boolean) || null,
    }
  })

  return {
    groups,
    activeGroupKey: groups.find((group) => group.isActive)?.key || null,
  }
}
