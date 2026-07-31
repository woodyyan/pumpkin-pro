function isPlainObject(value) {
  return Boolean(value) && typeof value === 'object' && !Array.isArray(value)
}

function stableStringify(value) {
  if (Array.isArray(value)) {
    return `[${value.map((item) => stableStringify(item)).join(',')}]`
  }
  if (isPlainObject(value)) {
    const keys = Object.keys(value).sort()
    return `{${keys.map((key) => `${JSON.stringify(key)}:${stableStringify(value[key])}`).join(',')}}`
  }
  return JSON.stringify(value)
}

export function buildDefaultSignalConfig(symbol, strategies = []) {
  return {
    symbol,
    strategy_id: strategies[0]?.id || '',
    is_enabled: false,
    cooldown_seconds: 3600,
    eval_interval_seconds: 3600,
    thresholds: {},
    // 持仓感知默认开启（只影响"是否推送无意义信号"，无交易决策风险）；
    // 止损/移动止盈默认 0=关闭（会强制发出卖出信号，必须由用户显式设置）。
    position_aware_enabled: true,
    max_position_pct: 0,
    max_add_times: 0,
    stop_loss_pct: 0,
    trailing_stop_pct: 0,
  }
}

function toNonNegativeNumber(value) {
  const num = Number(value)
  return Number.isFinite(num) && num > 0 ? num : 0
}

export function normalizeSignalConfig(input, symbol, strategies = []) {
  if (!input) return buildDefaultSignalConfig(symbol, strategies)
  return {
    symbol: input.symbol || symbol,
    strategy_id: typeof input.strategy_id === 'string' ? input.strategy_id : '',
    is_enabled: Boolean(input.is_enabled),
    cooldown_seconds: Number(input.cooldown_seconds) > 0 ? Number(input.cooldown_seconds) : 3600,
    eval_interval_seconds: Number(input.eval_interval_seconds) > 0 ? Number(input.eval_interval_seconds) : 3600,
    thresholds: isPlainObject(input.thresholds) ? input.thresholds : {},
    // 后端未返回该字段时（老数据）默认视为开启，与后端默认值保持一致。
    position_aware_enabled: input.position_aware_enabled === undefined ? true : Boolean(input.position_aware_enabled),
    max_position_pct: toNonNegativeNumber(input.max_position_pct),
    max_add_times: toNonNegativeNumber(input.max_add_times),
    stop_loss_pct: toNonNegativeNumber(input.stop_loss_pct),
    trailing_stop_pct: toNonNegativeNumber(input.trailing_stop_pct),
  }
}

export function hasSignalConfigChanged(serverConfig, draftConfig) {
  if (!serverConfig && !draftConfig) return false
  if (!serverConfig || !draftConfig) return true
  return (
    (serverConfig.symbol || '') !== (draftConfig.symbol || '') ||
    (serverConfig.strategy_id || '') !== (draftConfig.strategy_id || '') ||
    Boolean(serverConfig.is_enabled) !== Boolean(draftConfig.is_enabled) ||
    Number(serverConfig.cooldown_seconds || 0) !== Number(draftConfig.cooldown_seconds || 0) ||
    Number(serverConfig.eval_interval_seconds || 0) !== Number(draftConfig.eval_interval_seconds || 0) ||
    Boolean(serverConfig.position_aware_enabled) !== Boolean(draftConfig.position_aware_enabled) ||
    Number(serverConfig.max_position_pct || 0) !== Number(draftConfig.max_position_pct || 0) ||
    Number(serverConfig.max_add_times || 0) !== Number(draftConfig.max_add_times || 0) ||
    Number(serverConfig.stop_loss_pct || 0) !== Number(draftConfig.stop_loss_pct || 0) ||
    Number(serverConfig.trailing_stop_pct || 0) !== Number(draftConfig.trailing_stop_pct || 0) ||
    stableStringify(serverConfig.thresholds || {}) !== stableStringify(draftConfig.thresholds || {})
  )
}

export function canEnableSignal(config) {
  if (!config?.strategy_id) {
    return { ok: false, reason: '请先选择策略，再开启信号' }
  }
  return { ok: true, reason: '' }
}

export function buildSignalStatusSummary({ config, isToggling = false, toggleTargetEnabled = null } = {}) {
  const enabled = Boolean(config?.is_enabled)
  if (isToggling) {
    return toggleTargetEnabled ? '交易信号开启中...' : '交易信号关闭中...'
  }
  return enabled ? '交易信号已开启' : '交易信号已关闭'
}

export function buildSignalConfigMeta({ config, strategyMap = {}, isDirty = false, webhookConfigured = false, webhookEnabled = false } = {}) {
  const enabled = Boolean(config?.is_enabled)
  const strategyName = strategyMap?.[config?.strategy_id]?.name || '未选择策略'
  const intervalSeconds = Number(config?.eval_interval_seconds) || 3600
  const intervalLabelMap = {
    900: '每 15 分钟',
    1800: '每 30 分钟',
    3600: '每小时',
    7200: '每 2 小时',
    14400: '每 4 小时',
  }
  const intervalLabel = intervalLabelMap[intervalSeconds] || `每 ${Math.max(1, Math.round(intervalSeconds / 60))} 分钟`

  return [
    { label: '状态', value: enabled ? '已开启' : '已关闭' },
    { label: '策略', value: strategyName },
    { label: '频率', value: intervalLabel },
    { label: '推送', value: enabled ? ((webhookConfigured && webhookEnabled) ? '已就绪' : '未就绪') : '未启用' },
    { label: '持仓感知', value: buildPositionAwareSummary(config) },
    ...(isDirty ? [{ label: '配置', value: '有未保存修改', tone: 'warning' }] : []),
  ]
}

/**
 * 概括持仓感知与风控的启用情况，让用户一眼知道当前有哪些门控在生效。
 */
export function buildPositionAwareSummary(config) {
  if (config?.position_aware_enabled === false) return '已关闭'
  const parts = []
  const maxPosition = toNonNegativeNumber(config?.max_position_pct)
  const stopLoss = toNonNegativeNumber(config?.stop_loss_pct)
  const trailingStop = toNonNegativeNumber(config?.trailing_stop_pct)
  const maxAddTimes = toNonNegativeNumber(config?.max_add_times)
  if (maxPosition > 0) parts.push(`单票上限 ${maxPosition}%`)
  if (stopLoss > 0) parts.push(`止损 ${stopLoss}%`)
  if (trailingStop > 0) parts.push(`移动止盈 ${trailingStop}%`)
  if (maxAddTimes > 0) parts.push(`加仓上限 ${maxAddTimes} 次`)
  return parts.length > 0 ? `已开启（${parts.join('、')}）` : '已开启'
}
export function buildSignalConfigPayload(config, enabled = config?.is_enabled) {
  return {
    strategy_id: config?.strategy_id || '',
    is_enabled: Boolean(enabled),
    cooldown_seconds: Number(config?.cooldown_seconds) || 3600,
    eval_interval_seconds: Number(config?.eval_interval_seconds) || 3600,
    thresholds: isPlainObject(config?.thresholds) ? config.thresholds : {},
    // 风控字段始终显式下发：后端用指针区分"未传保持原值"与"显式 0 关闭规则"，
    // 这里全部传值，保证用户把某项调回 0（关闭）时能真正落库。
    position_aware_enabled: config?.position_aware_enabled === undefined ? true : Boolean(config.position_aware_enabled),
    max_position_pct: toNonNegativeNumber(config?.max_position_pct),
    max_add_times: toNonNegativeNumber(config?.max_add_times),
    stop_loss_pct: toNonNegativeNumber(config?.stop_loss_pct),
    trailing_stop_pct: toNonNegativeNumber(config?.trailing_stop_pct),
  }
}

// ── 持仓感知门控：信号历史展示辅助 ──

const GATE_DECISION_LABELS = {
  pass: '已推送',
  suppressed: '已归档（未推送）',
  overridden: '风控强制',
}

/**
 * 描述一条信号的门控状态，供信号历史列表展示。
 * 设计要求：被拦截的信号不能隐藏，必须弱化展示并给出可读原因（静默必须可解释）。
 */
export function describeSignalGate(event) {
  if (!event) return null
  const decision = String(event.gate_decision || '').trim()
  // 老数据没有门控字段：按已推送处理，不显示门控信息。
  if (!decision) {
    return { decision: 'pass', label: '已推送', tone: 'normal', isSuppressed: false, message: '', semanticLabel: '' }
  }
  const isSuppressed = decision === 'suppressed'
  const isOverridden = decision === 'overridden'
  return {
    decision,
    label: GATE_DECISION_LABELS[decision] || '已推送',
    // muted = 弱化但不隐藏；warning = 风控强制，需要醒目。
    tone: isSuppressed ? 'muted' : isOverridden ? 'warning' : 'normal',
    isSuppressed,
    isOverridden,
    message: event.suppressed_message || event.reason?.gate_message || '',
    semanticLabel: event.semantic_label || event.reason?.semantic_label || '',
    rawSide: event.raw_side || '',
    finalSide: event.final_side || event.side || '',
    // 策略原始方向被风控改写时需要在 UI 明示，避免用户困惑。
    isSideRewritten: Boolean(event.raw_side && event.final_side && event.raw_side !== event.final_side),
  }
}

/**
 * 把持仓快照整理成「标签 + 值」列表，用于信号详情展示。
 * 关键：未录持仓与确认空仓必须区分表述，避免用户误解。
 */
export function buildPositionSnapshotRows(snapshot) {
  if (!snapshot || typeof snapshot !== 'object') return []
  const status = String(snapshot.data_status || '')
  if (status === 'unknown') {
    return [{ label: '持仓', value: '未录入' }]
  }
  const shares = Number(snapshot.shares) || 0
  if (shares <= 0) {
    return [{ label: '持仓', value: '当前空仓' }]
  }
  const rows = [
    { label: '持仓数量', value: `${shares} 股` },
    { label: '持仓成本', value: formatPrice(snapshot.avg_cost_price) },
  ]
  const pnlPct = Number(snapshot.unrealized_pnl_pct)
  if (Number.isFinite(pnlPct) && snapshot.avg_cost_price > 0) {
    rows.push({
      label: '浮动盈亏',
      value: `${pnlPct >= 0 ? '+' : ''}${pnlPct.toFixed(2)}%`,
      // A 股口径：盈利红、亏损绿。
      tone: pnlPct >= 0 ? 'up' : 'down',
    })
  }
  const weight = Number(snapshot.position_weight_pct)
  if (Number.isFinite(weight) && weight > 0) {
    rows.push({ label: '仓位占比', value: `${weight.toFixed(2)}%` })
  }
  if (status === 'stale') {
    rows.push({ label: '数据', value: '较久未更新', tone: 'warning' })
  }
  return rows
}

function formatPrice(value) {
  const num = Number(value)
  if (!Number.isFinite(num) || num <= 0) return '—'
  return num.toFixed(3)
}

export function mergeServerSignalConfig({ serverConfig, draftConfig, isDirty, isToggling }) {
  if (!serverConfig) return draftConfig
  if (isDirty || isToggling) return draftConfig || serverConfig
  return serverConfig
}
