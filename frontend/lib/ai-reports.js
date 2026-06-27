export const AI_REPORT_PRICING_PLANS = [
  {
    key: 'trial',
    name: '体验版',
    price: '9.9 元',
    quota: '1 份',
    reportType: '标准 PDF 报告',
    description: '适合先体验一只个股的 AI研报交付质量。',
    highlights: ['1 份标准 PDF 报告', '基础个股分析', '风险提示与操作参考'],
  },
  {
    key: 'starter',
    name: '入门版',
    price: '39 元',
    quota: '5 份',
    reportType: '标准 PDF 报告',
    description: '适合批量查看自选股，单份成本更低。',
    highlights: ['5 份标准 PDF 报告', '覆盖多只自选股', '适合初步筛选机会'],
  },
  {
    key: 'investment',
    name: '投资版',
    price: '69 元',
    quota: '10 份',
    reportType: '增值分析服务',
    description: '适合结合个人持仓和具体问题做更有针对性的分析。',
    highlights: ['10 份 AI研报', '可定制投资周期：短线 / 波段 / 长线', '支持结合用户持仓情况分析', '支持自定义分析问题'],
    examples: ['我的成本 42 元，要不要止损？', '未来一年最大的风险是什么？', '为什么该股票近一个月一直跌？', '现在该股票还能加仓吗？'],
    badge: '推荐',
    featured: true,
  },
  {
    key: 'professional',
    name: '专业版',
    price: '199 元',
    quota: '50 份',
    reportType: '专业跟踪服务',
    description: '适合高频研究、组合跟踪和多股票横向比较。',
    highlights: ['50 份 AI研报', '包含投资版全部服务', '优先生成，免排队', '多股票对比分析', '新功能优先体验'],
    examples: ['帮我比较小米 vs 比亚迪 vs 宁德时代的股票'],
    badge: '最高性价比',
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
