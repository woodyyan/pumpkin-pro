const FACTOR_ORDER = ['value', 'dividend_yield', 'growth', 'quality', 'momentum', 'size', 'low_volatility']

const STATUS_META = {
  completed: { label: '正常', toneClass: 'bg-negative/10 text-negative' },
  partial: { label: '部分数据', toneClass: 'bg-amber-500/10 text-amber-700' },
  failed: { label: '计算失败', toneClass: 'bg-positive/10 text-positive' },
  pending: { label: '生成中', toneClass: 'bg-[var(--color-bg-hover)] text-foreground-muted' },
}

function buildStockPoolState(payload) {
  const lists = Array.isArray(payload?.lists) ? payload.lists.map(normalizeList).filter(Boolean) : []
  lists.sort((left, right) => factorSortIndex(left.factorKey) - factorSortIndex(right.factorKey))
  return {
    sourceTradeDate: text(payload?.source_trade_date),
    marketStatus: text(payload?.market_status) || 'closed',
    priceLabel: text(payload?.price_label) || '最近收盘价',
    quoteUpdatedAt: text(payload?.quote_updated_at),
    quoteStatus: text(payload?.quote_status) || 'unavailable',
    lists,
  }
}

function normalizeList(item) {
  if (!item || !text(item.factor_key)) return null
  const status = text(item.status).toLowerCase() || 'pending'
  const meta = STATUS_META[status] || STATUS_META.pending
  return {
    indexId: text(item.index_id),
    factorKey: text(item.factor_key),
    name: text(item.name) || '--',
    sourceTradeDate: text(item.source_trade_date),
    rebalanceDate: text(item.rebalance_date),
    effectiveStartDate: text(item.effective_start_date),
    status,
    statusLabel: meta.label,
    statusToneClass: meta.toneClass,
    warningText: text(item.warning_text),
    currentConstituentCount: asNumber(item.current_constituent_count, 0),
    items: Array.isArray(item.items) ? item.items.map(normalizeItem).filter(Boolean) : [],
  }
}

function normalizeItem(item) {
  if (!item || !text(item.code)) return null
  const quote = item.quote || {}
  return {
    rank: asNumber(item.rank, 0),
    code: text(item.code),
    symbol: text(item.symbol),
    name: text(item.name) || text(item.code),
    exchange: text(item.exchange),
    industry: text(item.industry),
    factorScore: asNumber(item.factor_score, null),
    signalClosePrice: asNumber(item.signal_close_price, null),
    quote: {
      lastPrice: asNumber(quote.last_price, null),
      changeRate: asNumber(quote.change_rate, null),
      turnover: asNumber(quote.turnover, null),
      updatedAt: text(quote.updated_at),
      status: text(quote.status) || 'unavailable',
    },
  }
}

function factorSortIndex(key) {
  const index = FACTOR_ORDER.indexOf(text(key))
  return index >= 0 ? index : FACTOR_ORDER.length
}

function text(value) {
  return String(value || '').trim()
}

function asNumber(value, fallback) {
  const numeric = Number(value)
  return Number.isFinite(numeric) ? numeric : fallback
}

export {
  buildStockPoolState,
  normalizeList,
  normalizeItem,
}
