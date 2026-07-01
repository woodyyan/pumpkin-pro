export const STOCK_DETAIL_TAB_KEYS = Object.freeze({
  OVERVIEW: 'overview',
  CHART: 'chart',
  TECHNICAL: 'technical',
  FUNDAMENTAL: 'fundamental',
  AI: 'ai',
  PORTFOLIO: 'portfolio',
})

export const STOCK_DETAIL_TABS = Object.freeze([
  {
    key: STOCK_DETAIL_TAB_KEYS.OVERVIEW,
    label: '概览',
    shortLabel: '概览',
    mobileGroup: 'overview',
    description: '行情快照、AI入口、新闻与关键摘要',
  },
  {
    key: STOCK_DETAIL_TAB_KEYS.CHART,
    label: '走势',
    shortLabel: '走势',
    mobileGroup: 'chart',
    description: '历史走势、区间收益、个股与大盘对比、异动',
  },
  {
    key: STOCK_DETAIL_TAB_KEYS.TECHNICAL,
    label: '技术',
    shortLabel: '技术',
    mobileGroup: 'analysis',
    description: '均线、RSI、MACD、布林带、支撑与压力',
  },
  {
    key: STOCK_DETAIL_TAB_KEYS.FUNDAMENTAL,
    label: '基本面',
    shortLabel: '基本',
    mobileGroup: 'analysis',
    description: '估值、盈利质量、收入与利润概览',
  },
  {
    key: STOCK_DETAIL_TAB_KEYS.AI,
    label: 'AI & 资讯',
    shortLabel: 'AI',
    mobileGroup: 'analysis',
    description: 'AI分析结果、历史观点与个股资讯',
  },
  {
    key: STOCK_DETAIL_TAB_KEYS.PORTFOLIO,
    label: '持仓 & 提醒',
    shortLabel: '持仓',
    mobileGroup: 'portfolio',
    description: '个人持仓、交易记录与信号提醒',
    requiresLogin: true,
  },
])

const STOCK_DETAIL_TAB_KEY_SET = new Set(STOCK_DETAIL_TABS.map((tab) => tab.key))

export function normalizeStockDetailTab(tab) {
  const value = Array.isArray(tab) ? tab[0] : tab
  if (typeof value !== 'string') return STOCK_DETAIL_TAB_KEYS.OVERVIEW
  const normalized = value.trim().toLowerCase()
  return STOCK_DETAIL_TAB_KEY_SET.has(normalized) ? normalized : STOCK_DETAIL_TAB_KEYS.OVERVIEW
}

export function getStockDetailTabByKey(tab) {
  const key = normalizeStockDetailTab(tab)
  return STOCK_DETAIL_TABS.find((item) => item.key === key) || STOCK_DETAIL_TABS[0]
}

export function getStockDetailMobileGroups(tabs = STOCK_DETAIL_TABS) {
  const groups = []
  const seen = new Set()
  for (const tab of tabs) {
    const groupKey = tab?.mobileGroup || tab?.key
    if (!groupKey || seen.has(groupKey)) continue
    seen.add(groupKey)
    if (groupKey === 'analysis') {
      groups.push({ key: groupKey, label: '分析', shortLabel: '分析' })
    } else if (groupKey === 'portfolio') {
      groups.push({ key: groupKey, label: '持仓', shortLabel: '持仓' })
    } else {
      groups.push({ key: tab.key, label: tab.shortLabel || tab.label, shortLabel: tab.shortLabel || tab.label })
    }
  }
  return groups
}

export function isStockDetailTabInMobileGroup(tabKey, groupKey) {
  const tab = getStockDetailTabByKey(tabKey)
  if (!groupKey) return false
  return (tab.mobileGroup || tab.key) === groupKey || tab.key === groupKey
}
