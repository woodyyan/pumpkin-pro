import {
  buildInitialStrategyParams,
  sanitizeStrategyParams,
} from './strategy-form';

export const STRATEGY_PRESETS = [
  {
    typeKey: 'trend_cross',
    implementationKey: 'trend_cross',
    typeLabel: '趋势跟踪（双均线）',
    shortLabel: '趋势跟踪',
    category: '趋势',
    namePrefix: '趋势跟踪策略',
    idPrefix: 'trend-strategy',
    keyPrefix: 'trend_strategy',
    defaultDescription: '策略逻辑：短均线向上突破长均线时买入，短均线向下跌破长均线时卖出，适合单边趋势更明确的行情。示例：短均线=20、长均线=60，当 MA20 从下方向上穿越 MA60 触发买入；后续 MA20 再次跌破 MA60 时触发卖出。',
    legacyDescriptions: ['短均线上穿长均线买入，下穿卖出，适合趋势型行情。'],
    paramSchema: [
      {
        key: 'ma_short',
        label: '短均线周期',
        type: 'integer',
        required: true,
        default: 20,
        min: 2,
        max: 250,
        step: 1,
        description: '用于捕捉短期趋势变化。',
        options: [],
      },
      {
        key: 'ma_long',
        label: '长均线周期',
        type: 'integer',
        required: true,
        default: 60,
        min: 3,
        max: 500,
        step: 1,
        description: '用于识别长期趋势方向。',
        options: [],
      },
    ],
    requiredIndicators: [
      { type: 'ma', params: ['ma_short', 'ma_long'] },
    ],
    chartOverlays: [
      { type: 'line', template: 'MA{ma_short}' },
      { type: 'line', template: 'MA{ma_long}' },
    ],
    uiSchema: { param_order: ['ma_short', 'ma_long'] },
    executionOptions: {},
    metadata: { aliases: ['趋势跟踪（双均线）'] },
  },
  {
    typeKey: 'grid',
    implementationKey: 'grid',
    typeLabel: '网格交易',
    shortLabel: '网格交易',
    category: '震荡',
    namePrefix: '网格交易策略',
    idPrefix: 'grid-strategy',
    keyPrefix: 'grid_strategy',
    defaultDescription: '策略逻辑：围绕基准价按固定步长上下分层挂单，价格下探逐级买入、价格反弹逐级卖出，适合区间震荡市场。示例：基准价 100、网格数量=5、步长=3%，可在 97/94/91 分层买入，在 103/106/109 分层止盈。',
    legacyDescriptions: ['围绕基准价分层挂单，适合震荡市场分批低买高卖。'],
    paramSchema: [
      {
        key: 'grid_count',
        label: '网格数量',
        type: 'integer',
        required: true,
        default: 5,
        min: 2,
        max: 20,
        step: 1,
        description: '决定买卖网格层数。',
        options: [],
      },
      {
        key: 'grid_step',
        label: '网格步长',
        type: 'number',
        required: true,
        default: 0.05,
        min: 0.001,
        max: 0.5,
        step: 0.001,
        description: '相邻网格的价格间距比例。',
        options: [],
      },
    ],
    requiredIndicators: [],
    chartOverlays: [],
    uiSchema: { param_order: ['grid_count', 'grid_step'] },
    executionOptions: {},
    metadata: { aliases: [] },
  },
  {
    typeKey: 'bollinger_reversion',
    implementationKey: 'bollinger_reversion',
    typeLabel: '均值回归（布林带）',
    shortLabel: '均值回归',
    category: '均值回归',
    namePrefix: '均值回归策略',
    idPrefix: 'bollinger-strategy',
    keyPrefix: 'bollinger_strategy',
    defaultDescription: '策略逻辑：价格偏离布林带区间后，等待回归中轨的机会；常见做法是接近/跌破下轨时分批买入，接近/突破上轨时分批卖出。示例：周期=20、标准差=2，当价格触及下轨且出现止跌信号可尝试买入，反弹至中轨或上轨附近逐步止盈。',
    legacyDescriptions: ['价格跌破下轨买入、突破上轨卖出，捕捉回归均值机会。'],
    paramSchema: [
      {
        key: 'bb_period',
        label: '布林带周期',
        type: 'integer',
        required: true,
        default: 20,
        min: 5,
        max: 250,
        step: 1,
        description: '用于计算布林带中轨的均线周期。',
        options: [],
      },
      {
        key: 'bb_std',
        label: '标准差倍数',
        type: 'number',
        required: true,
        default: 2,
        min: 0.1,
        max: 5,
        step: 0.1,
        description: '用于计算布林带上下轨宽度。',
        options: [],
      },
    ],
    requiredIndicators: [
      { type: 'bollinger', params: ['bb_period', 'bb_std'] },
    ],
    chartOverlays: [
      { type: 'line', template: 'BB_upper' },
      { type: 'line', template: 'BB_mid' },
      { type: 'line', template: 'BB_lower' },
    ],
    uiSchema: { param_order: ['bb_period', 'bb_std'] },
    executionOptions: {},
    metadata: { aliases: ['均值回归（布林带）'] },
  },
  {
    typeKey: 'rsi_range',
    implementationKey: 'rsi_range',
    typeLabel: '区间交易（RSI）',
    shortLabel: '区间交易',
    category: '区间',
    namePrefix: '区间交易策略',
    idPrefix: 'rsi-strategy',
    keyPrefix: 'rsi_strategy',
    defaultDescription: '策略逻辑：RSI 从低位阈值向上突破时视为超卖修复买点，RSI 从高位阈值向下跌破时视为超买回落卖点，适合箱体或弱趋势震荡。示例：RSI 周期=14、低位=30、高位=70，当 RSI 从 28 回升并上穿 30 触发买入；当 RSI 从 74 回落并跌破 70 触发卖出。',
    legacyDescriptions: ['RSI 从低位回升买入，从高位回落卖出，适合箱体行情。'],
    paramSchema: [
      {
        key: 'rsi_period',
        label: 'RSI 周期',
        type: 'integer',
        required: true,
        default: 14,
        min: 2,
        max: 120,
        step: 1,
        description: '用于计算 RSI 指标的回看窗口。',
        options: [],
      },
      {
        key: 'rsi_low',
        label: '低位阈值',
        type: 'number',
        required: true,
        default: 30,
        min: 1,
        max: 50,
        step: 1,
        description: 'RSI 从低位线向上突破时触发买入。',
        options: [],
      },
      {
        key: 'rsi_high',
        label: '高位阈值',
        type: 'number',
        required: true,
        default: 70,
        min: 50,
        max: 99,
        step: 1,
        description: 'RSI 从高位线向下跌破时触发卖出。',
        options: [],
      },
    ],
    requiredIndicators: [
      { type: 'rsi', params: ['rsi_period'] },
    ],
    chartOverlays: [
      { type: 'line', template: 'RSI_{rsi_period}' },
    ],
    uiSchema: { param_order: ['rsi_period', 'rsi_low', 'rsi_high'] },
    executionOptions: {},
    metadata: { aliases: ['区间交易（RSI）'] },
  },
];

const PRESET_MAP = new Map(STRATEGY_PRESETS.map((preset) => [preset.typeKey, preset]));
const IMPLEMENTATION_MAP = new Map(STRATEGY_PRESETS.map((preset) => [preset.implementationKey, preset]));

export function getStrategyPresetByType(typeKey) {
  return PRESET_MAP.get(typeKey) || null;
}

export function getStrategyPresetByImplementation(implementationKey) {
  return IMPLEMENTATION_MAP.get(implementationKey) || null;
}

export function buildPresetDefinition(preset) {
  if (!preset) return null;
  return {
    implementation_key: preset.implementationKey,
    param_schema: preset.paramSchema,
    default_params: buildInitialStrategyParams({
      param_schema: preset.paramSchema,
      default_params: {},
      ui_schema: preset.uiSchema,
    }),
    ui_schema: preset.uiSchema,
  };
}

export function buildDraftFromStrategy(strategy) {
  const preset = getStrategyPresetByImplementation(strategy?.implementation_key);
  if (!preset) {
    throw new Error(`未识别的策略类型：${strategy?.implementation_key || 'unknown'}`);
  }

  return {
    id: strategy?.id || '',
    key: strategy?.key || '',
    name: strategy?.name || '',
    description: resolveStrategyDescription(strategy?.description, preset),
    category: strategy?.category || preset.category,
    status: strategy?.status || 'draft',
    version: strategy?.version || 1,
    typeKey: preset.typeKey,
    params: {
      ...buildInitialStrategyParams(strategy),
      ...(strategy?.default_params || {}),
    },
  };
}

export function createDraftFromType(typeKey, strategies) {
  const preset = getStrategyPresetByType(typeKey);
  if (!preset) {
    throw new Error(`未识别的策略类型：${typeKey}`);
  }

  const nextIndex = pickNextIndex(preset, strategies || []);
  return {
    id: `${preset.idPrefix}-${nextIndex}`,
    key: `${preset.keyPrefix}_${nextIndex}`,
    name: `${preset.namePrefix} ${nextIndex}`,
    description: preset.defaultDescription,
    category: preset.category,
    status: 'draft',
    version: 1,
    typeKey: preset.typeKey,
    params: buildInitialStrategyParams({
      param_schema: preset.paramSchema,
      default_params: {},
      ui_schema: preset.uiSchema,
    }),
  };
}

export function buildPayloadFromDraft(draft) {
  const preset = getStrategyPresetByType(draft?.typeKey);
  if (!preset) {
    throw new Error('未选择有效的策略类型。');
  }

  const definition = buildPresetDefinition(preset);
  const defaultParams = sanitizeStrategyParams(definition, draft?.params || {});

  return {
    id: (draft?.id || '').trim(),
    key: (draft?.key || '').trim(),
    name: (draft?.name || '').trim(),
    description: draft?.description || preset.defaultDescription,
    category: preset.category,
    implementation_key: preset.implementationKey,
    status: draft?.status || 'draft',
    version: Number(draft?.version || 1),
    param_schema: preset.paramSchema,
    default_params: defaultParams,
    required_indicators: preset.requiredIndicators,
    chart_overlays: preset.chartOverlays,
    ui_schema: preset.uiSchema,
    execution_options: preset.executionOptions,
    metadata: preset.metadata,
  };
}

export function resolveStrategyDescription(description, preset) {
  if (!preset) {
    return description || '';
  }

  const trimmed = (description || '').trim();
  if (!trimmed) {
    return preset.defaultDescription;
  }

  const legacy = new Set((preset.legacyDescriptions || []).map((item) => (item || '').trim()).filter(Boolean));
  if (legacy.has(trimmed)) {
    return preset.defaultDescription;
  }

  return trimmed;
}

function pickNextIndex(preset, strategies) {
  const usedIds = new Set(strategies.map((item) => item.id));
  const usedKeys = new Set(strategies.map((item) => item.key));
  const usedNames = new Set(strategies.map((item) => item.name));

  let index = 1;
  while (
    usedIds.has(`${preset.idPrefix}-${index}`)
    || usedKeys.has(`${preset.keyPrefix}_${index}`)
    || usedNames.has(`${preset.namePrefix} ${index}`)
  ) {
    index += 1;
  }
  return index;
}
