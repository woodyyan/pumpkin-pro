const FACTOR_INDEX_ORDER = ['value', 'dividend_yield', 'growth', 'quality', 'momentum', 'size', 'low_volatility']

const STATUS_META = {
  completed: { label: '正常', toneClass: 'bg-negative/10 text-negative' },
  partial: { label: '部分数据', toneClass: 'bg-amber-500/10 text-amber-700' },
  failed: { label: '计算失败', toneClass: 'bg-positive/10 text-positive' },
  pending: { label: '生成中', toneClass: 'bg-[var(--color-bg-hover)] text-foreground-muted' },
}

function buildFactorIndexState(overview) {
  const items = Array.isArray(overview?.items) ? overview.items.map((item) => normalizeFactorIndexItem(item)).filter(Boolean) : []
  items.sort((left, right) => factorSortIndex(left.factorKey) - factorSortIndex(right.factorKey))
  return {
    sourceTradeDate: String(overview?.source_trade_date || '').trim(),
    items,
  }
}

function normalizeFactorIndexItem(item) {
  if (!item || !item.factor_key) return null
  const status = String(item.status || 'pending').trim().toLowerCase()
  const meta = STATUS_META[status] || STATUS_META.pending
  return {
    indexId: String(item.index_id || '').trim(),
    factorKey: String(item.factor_key || '').trim(),
    name: String(item.name || '').trim() || '--',
    nav: toNumber(item.nav),
    dailyReturn: toNumber(item.daily_return),
    totalReturn: toNumber(item.total_return),
    weeklyReturn: toNumber(item.weekly_return),
    monthlyReturn: toNumber(item.monthly_return),
    threeMonthReturn: toNumber(item.three_month_return),
    halfYearReturn: toNumber(item.half_year_return),
    sourceTradeDate: String(item.source_trade_date || '').trim(),
    rebalanceDate: String(item.rebalance_date || '').trim(),
    effectiveStartDate: String(item.effective_start_date || '').trim(),
    constituentCount: Number(item.constituent_count || 0),
    status,
    statusLabel: meta.label,
    statusToneClass: meta.toneClass,
    warningText: String(item.warning_text || '').trim(),
    trend: mapFactorTrendPoints(item.trend_points),
  }
}

function mapFactorTrendPoints(points) {
  if (!Array.isArray(points)) return []
  return points
    .map((point, idx) => {
      if (!point || typeof point !== 'object') return null
      const date = String(point.date || `point-${idx + 1}`).trim()
      const count = toNumber(point.count)
      if (!date || !Number.isFinite(count)) return null
      return { date, count }
    })
    .filter(Boolean)
}

function factorSortIndex(key) {
  const index = FACTOR_INDEX_ORDER.indexOf(String(key || '').trim())
  return index >= 0 ? index : FACTOR_INDEX_ORDER.length
}

function toNumber(value) {
  const num = Number(value)
  return Number.isFinite(num) ? num : null
}

module.exports = {
  buildFactorIndexState,
  mapFactorTrendPoints,
  normalizeFactorIndexItem,
}
