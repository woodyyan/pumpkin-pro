function normalizeNumber(value) {
  return typeof value === 'number' && Number.isFinite(value) ? value : 0
}

function scopeLabel(scope) {
  switch (scope) {
    case 'ASHARE': return 'A股'
    case 'HKEX': return '港股'
    default: return '全部'
  }
}

function inferSingleMarketScope(summary) {
  const explicitScope = String(summary?.scope || '').toUpperCase()
  if (explicitScope === 'ASHARE' || explicitScope === 'HKEX') {
    return explicitScope
  }

  const firstBlockScope = String(summary?.amounts_by_market?.[0]?.scope || '').toUpperCase()
  if (firstBlockScope === 'ASHARE' || firstBlockScope === 'HKEX') {
    return firstBlockScope
  }

  return String(summary?.amounts?.currency_code || '').toUpperCase() === 'HKD' ? 'HKEX' : 'ASHARE'
}

export function buildPortfolioOverviewBlocks(summary) {
  if (!summary) return []

  if (summary.mixed_currency) {
    return Array.isArray(summary.amounts_by_market) ? summary.amounts_by_market : []
  }

  if (!summary.amounts) return []

  const scope = inferSingleMarketScope(summary)
  const currencyCode = summary.amounts.currency_code || (scope === 'HKEX' ? 'HKD' : 'CNY')
  const currencySymbol = summary.amounts.currency_symbol || (scope === 'HKEX' ? 'HK$' : '¥')

  return [{
    scope,
    scope_label: scopeLabel(scope),
    currency_code: currencyCode,
    currency_symbol: currencySymbol,
    market_value_amount: normalizeNumber(summary.amounts.market_value_amount),
    total_cost_amount: normalizeNumber(summary.amounts.total_cost_amount),
    unrealized_pnl_amount: normalizeNumber(summary.amounts.unrealized_pnl_amount),
    realized_pnl_amount: normalizeNumber(summary.amounts.realized_pnl_amount),
    total_pnl_amount: normalizeNumber(summary.amounts.total_pnl_amount),
    today_pnl_amount: normalizeNumber(summary.amounts.today_pnl_amount),
    position_count: Number(summary.position_count || 0),
    profit_position_count: Number(summary.profit_position_count || 0),
    loss_position_count: Number(summary.loss_position_count || 0),
    max_position_weight_ratio: normalizeNumber(summary.max_position_weight_ratio),
  }]
}
