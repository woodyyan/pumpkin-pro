export function sortParamSchema(definition) {
  if (!definition?.param_schema) return [];

  const schema = [...definition.param_schema];
  const order = definition?.ui_schema?.param_order || [];
  if (!Array.isArray(order) || order.length === 0) return schema;

  const orderIndex = new Map(order.map((key, index) => [key, index]));
  return schema.sort((a, b) => {
    const aIndex = orderIndex.has(a.key) ? orderIndex.get(a.key) : Number.MAX_SAFE_INTEGER;
    const bIndex = orderIndex.has(b.key) ? orderIndex.get(b.key) : Number.MAX_SAFE_INTEGER;
    return aIndex - bIndex;
  });
}

export function buildInitialStrategyParams(definition) {
  const params = {};
  sortParamSchema(definition).forEach((item) => {
    const defaultValue = definition?.default_params?.[item.key] ?? item.default ?? '';
    params[item.key] = defaultValue;
  });
  return params;
}

export function sanitizeStrategyParams(definition, values) {
  const sanitized = {};
  sortParamSchema(definition).forEach((item) => {
    const rawValue = values?.[item.key] ?? definition?.default_params?.[item.key] ?? item.default;
    sanitized[item.key] = coerceStrategyValue(item, rawValue);
  });
  return sanitized;
}

export function validateStrategyParams(definition, values) {
  if (!definition) return '请选择策略。';

  const schema = sortParamSchema(definition);
  for (const item of schema) {
    const rawValue = values?.[item.key];
    if ((rawValue === '' || rawValue === undefined || rawValue === null) && item.required) {
      return `${item.label}不能为空。`;
    }

    const coerced = coerceStrategyValue(item, rawValue);
    if ((item.type === 'integer' || item.type === 'number') && Number.isNaN(Number(coerced))) {
      return `${item.label}格式不正确。`;
    }

    if ((item.type === 'integer' || item.type === 'number') && item.min !== undefined && item.min !== null && Number(coerced) < Number(item.min)) {
      return `${item.label}不能小于 ${item.min}。`;
    }

    if ((item.type === 'integer' || item.type === 'number') && item.max !== undefined && item.max !== null && Number(coerced) > Number(item.max)) {
      return `${item.label}不能大于 ${item.max}。`;
    }
  }

  const sanitized = sanitizeStrategyParams(definition, values);
  if (definition.implementation_key === 'trend_cross' && Number(sanitized.ma_short) >= Number(sanitized.ma_long)) {
    return '双均线策略要求短均线周期小于长均线周期。';
  }

  if (definition.implementation_key === 'rsi_range' && Number(sanitized.rsi_low) >= Number(sanitized.rsi_high)) {
    return 'RSI 低阈值必须小于高阈值。';
  }

  return '';
}

export function coerceStrategyValue(item, rawValue) {
  if (item.type === 'integer') {
    return Number.parseInt(rawValue, 10);
  }

  if (item.type === 'number') {
    return Number(rawValue);
  }

  if (item.type === 'boolean') {
    if (typeof rawValue === 'boolean') return rawValue;
    return ['true', '1', 'yes', 'on'].includes(String(rawValue).toLowerCase());
  }

  return rawValue ?? '';
}

export function getInputAttributes(item) {
  if (item.type === 'integer') {
    return { type: 'number', step: item.step ?? 1, min: item.min, max: item.max };
  }

  if (item.type === 'number') {
    return { type: 'number', step: item.step ?? 0.01, min: item.min, max: item.max };
  }

  return { type: 'text' };
}

export function prettyJson(value) {
  return JSON.stringify(value ?? {}, null, 2);
}

export function parseJsonField(text, label, fallback) {
  const trimmed = (text || '').trim();
  if (!trimmed) return fallback;

  try {
    return JSON.parse(trimmed);
  } catch {
    throw new Error(`${label} 不是合法 JSON。`);
  }
}

export function buildStrategyDraft(strategy) {
  return {
    id: strategy?.id || '',
    key: strategy?.key || '',
    name: strategy?.name || '',
    description: strategy?.description || '',
    category: strategy?.category || '通用',
    implementation_key: strategy?.implementation_key || 'trend_cross',
    status: strategy?.status || 'draft',
    version: strategy?.version || 1,
    param_schema_text: prettyJson(strategy?.param_schema || []),
    default_params_text: prettyJson(strategy?.default_params || {}),
    required_indicators_text: prettyJson(strategy?.required_indicators || []),
    chart_overlays_text: prettyJson(strategy?.chart_overlays || []),
    ui_schema_text: prettyJson(strategy?.ui_schema || {}),
    execution_options_text: prettyJson(strategy?.execution_options || {}),
    metadata_text: prettyJson(strategy?.metadata || {}),
  };
}

export function createEmptyStrategyDraft(defaultImplementationKey = 'trend_cross') {
  return buildStrategyDraft({
    implementation_key: defaultImplementationKey,
    status: 'draft',
    version: 1,
    param_schema: [],
    default_params: {},
    required_indicators: [],
    chart_overlays: [],
    ui_schema: { param_order: [] },
    execution_options: {},
    metadata: {},
  });
}

export function buildStrategyPayloadFromDraft(draft) {
  return {
    id: (draft.id || '').trim(),
    key: (draft.key || '').trim(),
    name: (draft.name || '').trim(),
    description: draft.description || '',
    category: draft.category || '通用',
    implementation_key: draft.implementation_key || 'trend_cross',
    status: draft.status || 'draft',
    version: Number(draft.version || 1),
    param_schema: parseJsonField(draft.param_schema_text, '参数定义', []),
    default_params: parseJsonField(draft.default_params_text, '默认参数', {}),
    required_indicators: parseJsonField(draft.required_indicators_text, '指标配置', []),
    chart_overlays: parseJsonField(draft.chart_overlays_text, '图表叠加配置', []),
    ui_schema: parseJsonField(draft.ui_schema_text, 'UI 配置', {}),
    execution_options: parseJsonField(draft.execution_options_text, '执行配置', {}),
    metadata: parseJsonField(draft.metadata_text, '元数据', {}),
  };
}
