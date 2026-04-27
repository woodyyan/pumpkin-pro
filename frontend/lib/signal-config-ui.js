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
  }
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
    ...(isDirty ? [{ label: '配置', value: '有未保存修改', tone: 'warning' }] : []),
  ]
}
export function buildSignalConfigPayload(config, enabled = config?.is_enabled) {
  return {
    strategy_id: config?.strategy_id || '',
    is_enabled: Boolean(enabled),
    cooldown_seconds: Number(config?.cooldown_seconds) || 3600,
    eval_interval_seconds: Number(config?.eval_interval_seconds) || 3600,
    thresholds: isPlainObject(config?.thresholds) ? config.thresholds : {},
  }
}

export function mergeServerSignalConfig({ serverConfig, draftConfig, isDirty, isToggling }) {
  if (!serverConfig) return draftConfig
  if (isDirty || isToggling) return draftConfig || serverConfig
  return serverConfig
}
