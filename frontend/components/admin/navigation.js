export const ADMIN_NAV_ITEMS = [
  {
    key: 'overview',
    href: '/admin',
    label: '总览',
    shortLabel: '总览',
    description: '用户、流量、设备与核心指标',
  },
  {
    key: 'data',
    href: '/admin/data',
    label: '数据作业',
    shortLabel: '数据',
    description: '公司资料、因子流水线、四象限',
  },
  {
    key: 'ai',
    href: '/admin/ai',
    label: 'AI 管理',
    shortLabel: 'AI',
    description: '模型配置、AI 调用、AI 选股',
  },
  {
    key: 'ops',
    href: '/admin/ops',
    label: '运维与支持',
    shortLabel: '运维',
    description: '支付、备份、系统健康、反馈',
  },
]

export const ADMIN_TAB_ROUTE_MAP = {
  payments: '/admin/ops',
  backup: '/admin/ops',
  feedback: '/admin/ops',
  system: '/admin/ops',
  ai: '/admin/ai',
  aipicker: '/admin/ai',
  factor: '/admin/data',
  quadrant: '/admin/data',
  company_profiles: '/admin/data',
}

export function findAdminNavItem(section) {
  return ADMIN_NAV_ITEMS.find((item) => item.key === section) || ADMIN_NAV_ITEMS[0]
}

export function resolveAdminSectionFromPath(pathname = '') {
  if (pathname === '/admin' || pathname === '/admin/') return 'overview'
  const match = pathname.match(/^\/admin\/([^/?#]+)/)
  const candidate = match?.[1] || ''
  return ADMIN_NAV_ITEMS.some((item) => item.key === candidate) ? candidate : 'overview'
}
