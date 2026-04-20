export function isPortfolioPositionActive(item) {
  return Number(item?.shares || 0) > 0
}

export function getDefaultTradeDate() {
  const now = new Date()
  const year = now.getFullYear()
  const month = String(now.getMonth() + 1).padStart(2, '0')
  const day = String(now.getDate()).padStart(2, '0')
  return `${year}-${month}-${day}`
}

export function createPortfolioActionForm(action, item = null) {
  return {
    trade_date: getDefaultTradeDate(),
    quantity: '',
    price: '',
    fee_amount: '',
    avg_cost_price: action === 'adjust' ? String(item?.avg_cost_price ?? '') : '',
    note: '',
  }
}

function parseNumber(value) {
  if (value === null || value === undefined || value === '') return null
  const num = Number(value)
  return Number.isFinite(num) ? num : null
}

function currentPositionState(item) {
  const shares = Number(item?.shares || 0)
  const avgCostPrice = Number(item?.avg_cost_price || 0)
  const totalCostAmount = Number(item?.total_cost_amount || shares * avgCostPrice || 0)
  return {
    shares,
    avgCostPrice,
    totalCostAmount,
  }
}

export function buildPortfolioEventPreview(action, item, form) {
  const current = currentPositionState(item)
  const quantity = parseNumber(form?.quantity)
  const price = parseNumber(form?.price)
  const feeAmount = parseNumber(form?.fee_amount) ?? 0
  const avgCostPrice = parseNumber(form?.avg_cost_price)

  const preview = {
    valid: false,
    errors: [],
    nextShares: current.shares,
    nextAvgCostPrice: current.avgCostPrice,
    nextTotalCostAmount: current.totalCostAmount,
    realizedPnlAmount: 0,
    realizedPnlPct: 0,
  }

  switch (action) {
    case 'buy': {
      if (!(quantity > 0)) preview.errors.push('买入数量必须大于 0')
      if (!(price > 0)) preview.errors.push('买入价格必须大于 0')
      if (feeAmount < 0) preview.errors.push('手续费不能为负数')
      if (preview.errors.length > 0) return preview
      const buyCost = quantity * price + feeAmount
      preview.nextShares = current.shares + quantity
      preview.nextTotalCostAmount = current.totalCostAmount + buyCost
      preview.nextAvgCostPrice = preview.nextShares > 0 ? preview.nextTotalCostAmount / preview.nextShares : 0
      preview.valid = true
      return preview
    }
    case 'sell': {
      if (!(quantity > 0)) preview.errors.push('卖出数量必须大于 0')
      if (!(price > 0)) preview.errors.push('卖出价格必须大于 0')
      if (feeAmount < 0) preview.errors.push('手续费不能为负数')
      if (!(current.shares > 0)) preview.errors.push('当前无持仓，无法卖出')
      if (quantity > current.shares) preview.errors.push('卖出数量不能超过当前持仓')
      if (preview.errors.length > 0) return preview
      preview.nextShares = current.shares - quantity
      preview.nextAvgCostPrice = preview.nextShares > 0 ? current.avgCostPrice : 0
      preview.nextTotalCostAmount = preview.nextShares > 0 ? preview.nextShares * current.avgCostPrice : 0
      preview.realizedPnlAmount = quantity * (price - current.avgCostPrice) - feeAmount
      preview.realizedPnlPct = current.avgCostPrice > 0 ? ((price / current.avgCostPrice) - 1) * 100 : 0
      preview.valid = true
      return preview
    }
    case 'adjust': {
      if (!(current.shares > 0)) preview.errors.push('当前无持仓，无法调整均价')
      if (!(avgCostPrice > 0)) preview.errors.push('调整后的均价必须大于 0')
      if (!String(form?.note || '').trim()) preview.errors.push('调整均价请填写原因说明')
      if (preview.errors.length > 0) return preview
      preview.nextShares = current.shares
      preview.nextAvgCostPrice = avgCostPrice
      preview.nextTotalCostAmount = current.shares * avgCostPrice
      preview.valid = true
      return preview
    }
    default:
      preview.errors.push('未知操作类型')
      return preview
  }
}

function resolveCurrencySymbol(symbol) {
  return String(symbol || '').endsWith('.HK') ? 'HK$' : '¥'
}

function formatMoney(value, symbol) {
  if (typeof value !== 'number' || Number.isNaN(value)) return '--'
  const prefix = resolveCurrencySymbol(symbol)
  return `${prefix}${value.toLocaleString('zh-CN', { maximumFractionDigits: 2, minimumFractionDigits: 2 })}`
}

function formatSignedMoney(value, symbol) {
  if (typeof value !== 'number' || Number.isNaN(value)) return '--'
  const prefix = value > 0 ? '+' : ''
  return `${prefix}${formatMoney(value, symbol)}`
}

export function formatPortfolioEventHeadline(event, symbol) {
  if (!event) return '--'
  switch (event.event_type) {
    case 'buy':
      return `买入 ${Number(event.quantity || 0).toLocaleString('zh-CN')} 股 @ ${formatMoney(event.price || 0, symbol)}`
    case 'sell':
      return `卖出 ${Number(event.quantity || 0).toLocaleString('zh-CN')} 股 @ ${formatMoney(event.price || 0, symbol)}`
    case 'adjust_avg_cost':
      return '手动调整均价'
    case 'init':
      return '初始化持仓'
    case 'sync_position':
      return '校准当前持仓'
    default:
      return '持仓变动'
  }
}

export function formatPortfolioEventSubline(event, symbol) {
  if (!event) return '--'
  switch (event.event_type) {
    case 'buy':
      return `${Number(event.before_shares || 0).toLocaleString('zh-CN')} 股 → ${Number(event.after_shares || 0).toLocaleString('zh-CN')} 股 · 新均价 ${formatMoney(event.after_avg_cost_price || 0, symbol)}`
    case 'sell':
      return `${Number(event.before_shares || 0).toLocaleString('zh-CN')} 股 → ${Number(event.after_shares || 0).toLocaleString('zh-CN')} 股 · 已实现收益 ${formatSignedMoney(event.realized_pnl_amount || 0, symbol)}`
    case 'adjust_avg_cost':
      return `${formatMoney(event.before_avg_cost_price || 0, symbol)} → ${formatMoney(event.after_avg_cost_price || 0, symbol)} · 持仓 ${Number(event.after_shares || 0).toLocaleString('zh-CN')} 股未变`
    case 'init':
      return '由旧版持仓快照迁移生成'
    case 'sync_position':
      return `${Number(event.before_shares || 0).toLocaleString('zh-CN')} 股 → ${Number(event.after_shares || 0).toLocaleString('zh-CN')} 股 · 当前均价 ${formatMoney(event.after_avg_cost_price || 0, symbol)}`
    default:
      return '--'
  }
}

export function getPortfolioEventAccent(event) {
  if (!event) return 'text-white/70'
  if (event.event_type === 'buy') return 'text-rose-300'
  if (event.event_type === 'sell') return 'text-emerald-300'
  if (event.event_type === 'adjust_avg_cost') return 'text-sky-300'
  if (event.event_type === 'init' || event.event_type === 'sync_position') return 'text-amber-200'
  return 'text-white/70'
}
