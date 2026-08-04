// 信号中心视图模型纯函数。
// 页面组件只负责渲染与请求，状态归一、分组、徽章、筛选逻辑全部收敛在这里，便于单测。

// ── 订阅归一化 ──

export function normalizeSubscription(raw) {
  if (!raw || typeof raw !== 'object') return null
  return {
    id: raw.id || '',
    template_key: raw.template_key || '',
    template_name: raw.template_name || raw.template_key || '',
    category: raw.category || '',
    strategy_id: raw.strategy_id || '',
    scope_type: raw.scope_type || 'symbol',
    symbol: raw.symbol || '',
    symbol_name: raw.symbol_name || '',
    params: raw.params && typeof raw.params === 'object' ? raw.params : {},
    is_enabled: Boolean(raw.is_enabled),
    eval_interval_seconds: Number(raw.eval_interval_seconds) > 0 ? Number(raw.eval_interval_seconds) : 900,
    cooldown_seconds: Number(raw.cooldown_seconds) > 0 ? Number(raw.cooldown_seconds) : 3600,
    position_aware_enabled: raw.position_aware_enabled !== false,
    stop_loss_pct: Number(raw.stop_loss_pct) || 0,
    trailing_stop_pct: Number(raw.trailing_stop_pct) || 0,
  }
}

export function normalizeSubscriptions(list) {
  if (!Array.isArray(list)) return []
  return list.map(normalizeSubscription).filter(Boolean)
}

// ── 按股票分组 ──

export function groupSubscriptionsBySymbol(subscriptions) {
  const groups = []
  const index = new Map()
  for (const sub of normalizeSubscriptions(subscriptions)) {
    if (!sub.symbol) continue
    if (!index.has(sub.symbol)) {
      const group = { symbol: sub.symbol, symbol_name: sub.symbol_name || '', items: [], enabled_count: 0 }
      index.set(sub.symbol, group)
      groups.push(group)
    }
    const group = index.get(sub.symbol)
    group.items.push(sub)
    if (!group.symbol_name && sub.symbol_name) group.symbol_name = sub.symbol_name
    if (sub.is_enabled) group.enabled_count += 1
  }
  return groups
}

// ── 参数摘要 ──

// 用模板 param_schema 的 label/unit 渲染「目标价 100 元」式摘要。
export function buildParamsSummary(template, params) {
  if (!template || !Array.isArray(template.param_schema)) return ''
  const source = params && typeof params === 'object' ? params : {}
  const parts = []
  for (const field of template.param_schema) {
    const value = source[field.key] !== undefined ? source[field.key] : field.default
    if (value === undefined || value === null || value === '') continue
    parts.push(`${field.label} ${formatParamValue(value)}${field.unit || ''}`)
  }
  return parts.join(' · ')
}

function formatParamValue(value) {
  const num = Number(value)
  if (!Number.isFinite(num)) return String(value)
  return Number.isInteger(num) ? String(num) : String(Math.round(num * 100) / 100)
}

// ── 频率与状态文案 ──

export function evalIntervalLabel(seconds) {
  const map = { 900: '每 15 分钟', 1800: '每 30 分钟', 3600: '每小时', 7200: '每 2 小时', 14400: '每 4 小时' }
  return map[Number(seconds)] || `每 ${Math.max(1, Math.round(Number(seconds) / 60))} 分钟`
}

// 评估节奏说明：价格提醒走实时快照，指标/策略走日线试算+收盘确认。
export function evalModeLabel(subscription) {
  if (!subscription) return ''
  if (subscription.category === 'price_alert') return '盘中实时'
  if (subscription.template_key === 'volume_breakout') return '收盘确认'
  return evalIntervalLabel(subscription.eval_interval_seconds)
}

// bar_state → 徽章。tone 供组件映射语义化样式。
export function barStateMeta(barState) {
  switch (barState) {
    case 'intraday_provisional':
      return { label: '盘中试算', tone: 'warning' }
    case 'close_confirmed':
      return { label: '收盘确认', tone: 'info' }
    case 'realtime':
      return { label: '实时触发', tone: 'warning' }
    case 'test':
      return { label: '测试', tone: 'muted' }
    default:
      return { label: '', tone: 'muted' }
  }
}

// 方向徽章：A 股口径红涨绿跌（rise=红=买入提示，fall=绿=卖出提示；
// 组件层映射到 semantic token 时注意 negative=红、positive=绿）。
export function sideMeta(side) {
  const normalized = String(side || '').toUpperCase()
  if (normalized === 'BUY') return { label: '买入提示', tone: 'rise' }
  if (normalized === 'SELL') return { label: '卖出提示', tone: 'fall' }
  return { label: normalized || '—', tone: 'muted' }
}

// ── 事件归一化与筛选 ──

export function normalizeSignalEvent(raw) {
  if (!raw || typeof raw !== 'object') return null
  return {
    event_id: raw.event_id || '',
    symbol: raw.symbol || '',
    side: String(raw.side || '').toUpperCase(),
    bar_state: raw.bar_state || '',
    template_key: raw.template_key || '',
    template_name: raw.template_name || '',
    trade_date: raw.trade_date || '',
    is_read: Boolean(raw.is_read),
    is_delivered: Boolean(raw.is_delivered),
    gate_decision: raw.gate_decision || '',
    suppressed_message: raw.suppressed_message || '',
    semantic_label: raw.semantic_label || '',
    event_time: raw.event_time || '',
    // 幂等归一：同时接受原始 API 结构（reason.message）与已归一结构（message）。
    message: raw.reason?.message || raw.message || '',
    bar_state_note: raw.reason?.bar_state_note || raw.bar_state_note || '',
    disclaimer: raw.compliance?.disclaimer || raw.reason?.disclaimer || raw.disclaimer || '',
  }
}

export function normalizeSignalEvents(list) {
  if (!Array.isArray(list)) return []
  return list.map(normalizeSignalEvent).filter(Boolean)
}

export function filterSignalEvents(events, { symbol = '', side = '', barState = '' } = {}) {
  const normalizedSide = String(side || '').toUpperCase()
  return normalizeSignalEvents(events).filter((event) => {
    if (symbol && event.symbol !== symbol) return false
    if (normalizedSide && event.side !== normalizedSide) return false
    if (barState && event.bar_state !== barState) return false
    return true
  })
}

// ── 概览统计 ──

// todayCount 以 CST 自然日（trade_date）口径统计。
export function buildOverviewStats(subscriptions, events, unreadCount, todayTradeDate) {
  const subs = normalizeSubscriptions(subscriptions)
  const enabledCount = subs.filter((sub) => sub.is_enabled).length
  const todayEvents = normalizeSignalEvents(events).filter((event) => {
    if (todayTradeDate && event.trade_date) return event.trade_date === todayTradeDate
    return isSameCstDay(event.event_time)
  })
  return {
    enabledCount,
    totalCount: subs.length,
    todayCount: todayEvents.length,
    unreadCount: Math.max(0, Number(unreadCount) || 0),
  }
}

function isSameCstDay(isoTime) {
  if (!isoTime) return false
  const date = new Date(isoTime)
  if (Number.isNaN(date.getTime())) return false
  return cstDateString(date) === cstDateString(new Date())
}

export function cstDateString(date) {
  // UTC+8（A 股/港股同为东八区）。
  const shifted = new Date(date.getTime() + 8 * 3600 * 1000)
  return shifted.toISOString().slice(0, 10)
}

// ── 创建/更新载荷 ──

export function buildCreateSubscriptionPayload({ templateKey, symbol, strategyId = '', params = {}, evalIntervalSeconds = 900 }) {
  return {
    template_key: templateKey,
    scope_type: 'symbol',
    symbol,
    strategy_id: strategyId,
    params,
    is_enabled: true,
    eval_interval_seconds: evalIntervalSeconds,
  }
}

// schema 驱动的参数表单初始值：模板默认值。
export function buildTemplateParamDraft(template) {
  const draft = {}
  if (!template || !Array.isArray(template.param_schema)) return draft
  for (const field of template.param_schema) {
    if (field.default !== undefined) {
      draft[field.key] = field.default
    } else if (template.default_params && template.default_params[field.key] !== undefined) {
      draft[field.key] = template.default_params[field.key]
    }
  }
  return draft
}

// 表单校验：required + min/max。返回错误信息（空串=通过）。
export function validateTemplateParamDraft(template, draft) {
  if (!template || !Array.isArray(template.param_schema)) return ''
  for (const field of template.param_schema) {
    const raw = draft?.[field.key]
    if (raw === undefined || raw === null || raw === '') {
      if (field.required) return `请填写「${field.label}」`
      continue
    }
    const value = Number(raw)
    if (!Number.isFinite(value)) return `「${field.label}」必须是数字`
    if (field.min !== undefined && value < field.min) return `「${field.label}」不能小于 ${field.min}`
    if (field.max !== undefined && value > field.max) return `「${field.label}」不能大于 ${field.max}`
  }
  return ''
}

// 模板按分类分组（新增订阅弹窗用）。
export function groupTemplatesByCategory(templates) {
  const order = [
    { key: 'price_alert', label: '价格提醒' },
    { key: 'indicator', label: '指标信号' },
    { key: 'strategy', label: '策略信号' },
  ]
  const grouped = order.map((entry) => ({ ...entry, items: [] }))
  for (const template of Array.isArray(templates) ? templates : []) {
    const bucket = grouped.find((entry) => entry.key === template.category)
    if (bucket) bucket.items.push(template)
  }
  return grouped.filter((entry) => entry.items.length > 0)
}
