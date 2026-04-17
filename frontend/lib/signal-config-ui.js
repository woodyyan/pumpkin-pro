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
