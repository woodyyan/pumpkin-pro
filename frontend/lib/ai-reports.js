export const AI_REPORT_PRICING_PLANS = [
  {
    key: 'single',
    name: '单次体验',
    price: '9.9 元',
    quota: '1 份',
    unitPrice: '9.9 元/份',
    description: '适合先体验一只个股研报',
  },
  {
    key: 'standard',
    name: '标准套餐',
    price: '39 元',
    quota: '5 份',
    unitPrice: '7.8 元/份',
    description: '适合跟踪多只自选股',
    badge: '推荐',
  },
  {
    key: 'tracking',
    name: '深度跟踪',
    price: '69 元',
    quota: '10 份',
    unitPrice: '6.9 元/份',
    description: '适合高频复盘和组合观察',
  },
]

export const AI_REPORT_MARKET_OPTIONS = [
  { value: 'SSE', label: 'A股 · 上交所' },
  { value: 'SZSE', label: 'A股 · 深交所' },
  { value: 'HKEX', label: '中国香港股票' },
]

export const DEFAULT_AI_REPORT_DELIVERY_TEXT = '研报生成时间通常为 10 分钟到 24 小时不等，大部分情况下会在 1 小时内完成交付。具体时间取决于股票复杂度、数据完整度和人工复核情况。'

export const DEFAULT_AI_REPORT_RISK_DISCLAIMER = 'AI研报内容包含对个股的研究分析和投资建议，仅供投资研究参考，不构成收益承诺。证券市场存在风险，投资者应结合自身风险承受能力独立判断并审慎决策。'

export function getAIReportMarketLabel(exchange) {
  const normalized = String(exchange || '').toUpperCase()
  return AI_REPORT_MARKET_OPTIONS.find((item) => item.value === normalized)?.label || normalized || '--'
}

export function normalizeAIReportItems(items) {
  if (!Array.isArray(items)) return []
  return items.map((item) => ({
    id: String(item?.id || ''),
    stockName: String(item?.stock_name || '').trim(),
    symbol: String(item?.symbol || '').trim(),
    exchange: String(item?.exchange || '').trim().toUpperCase(),
    sourceTradeDate: String(item?.source_trade_date || '').trim(),
    thumbnailURL: String(item?.thumbnail_url || '').trim(),
  })).filter((item) => item.id && item.stockName && item.symbol)
}
