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
        label: '短均线周期（天）',
        type: 'integer',
        required: true,
        default: 20,
        min: 2,
        max: 250,
        step: 1,
        description: '用于捕捉短期趋势变化，单位：天。',
        options: [],
      },
      {
        key: 'ma_long',
        label: '长均线周期（天）',
        type: 'integer',
        required: true,
        default: 60,
        min: 3,
        max: 500,
        step: 1,
        description: '用于识别长期趋势方向，单位：天。',
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
        label: '网格数量（层）',
        type: 'integer',
        required: true,
        default: 5,
        min: 2,
        max: 20,
        step: 1,
        description: '决定买卖网格层数，单位：层。',
        options: [],
      },
      {
        key: 'grid_step',
        label: '网格步长（比例）',
        type: 'number',
        required: true,
        default: 0.05,
        min: 0.001,
        max: 0.5,
        step: 0.001,
        description: '相邻网格的价格间距比例，单位：比例（例如 0.03 = 3%）。',
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
        label: '布林带周期（天）',
        type: 'integer',
        required: true,
        default: 20,
        min: 5,
        max: 250,
        step: 1,
        description: '用于计算布林带中轨的均线周期，单位：天。',
        options: [],
      },
      {
        key: 'bb_std',
        label: '标准差倍数（倍）',
        type: 'number',
        required: true,
        default: 2,
        min: 0.1,
        max: 5,
        step: 0.1,
        description: '用于计算布林带上下轨宽度，单位：倍。',
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
        label: 'RSI 周期（天）',
        type: 'integer',
        required: true,
        default: 14,
        min: 2,
        max: 120,
        step: 1,
        description: '用于计算 RSI 指标的回看窗口，单位：天。',
        options: [],
      },
      {
        key: 'rsi_low',
        label: '低位阈值（RSI 点）',
        type: 'number',
        required: true,
        default: 30,
        min: 1,
        max: 50,
        step: 1,
        description: 'RSI 从低位线向上突破时触发买入，单位：RSI 点（0-100）。',
        options: [],
      },
      {
        key: 'rsi_high',
        label: '高位阈值（RSI 点）',
        type: 'number',
        required: true,
        default: 70,
        min: 50,
        max: 99,
        step: 1,
        description: 'RSI 从高位线向下跌破时触发卖出，单位：RSI 点（0-100）。',
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
  {
    typeKey: 'macd_cross',
    implementationKey: 'macd_cross',
    typeLabel: 'MACD 趋势策略',
    shortLabel: 'MACD',
    category: '趋势',
    namePrefix: 'MACD 策略',
    idPrefix: 'macd-strategy',
    keyPrefix: 'macd_strategy',
    defaultDescription: '策略逻辑：当 MACD 指标的 DIF 线（快线）从下方向上穿越 DEA 线（信号线）时形成金叉买入；DIF 从上方向下跌破 DEA 时形成死叉卖出。MACD 对趋势加速与减速更敏感，信号比双均线更平滑。示例：快线=12、慢线=26、信号线=9（经典参数），当 DIF 上穿 DEA 触发买入。',
    legacyDescriptions: [],
    paramSchema: [
      {
        key: 'fast_period',
        label: '快线周期（天）',
        type: 'integer',
        required: true,
        default: 12,
        min: 2,
        max: 50,
        step: 1,
        description: 'EMA 快线周期，用于计算 DIF，单位：天。',
        options: [],
      },
      {
        key: 'slow_period',
        label: '慢线周期（天）',
        type: 'integer',
        required: true,
        default: 26,
        min: 5,
        max: 100,
        step: 1,
        description: 'EMA 慢线周期，用于计算 DIF，单位：天。',
        options: [],
      },
      {
        key: 'signal_period',
        label: '信号线周期（天）',
        type: 'integer',
        required: true,
        default: 9,
        min: 2,
        max: 30,
        step: 1,
        description: 'DEA（信号线）的 EMA 平滑周期，单位：天。',
        options: [],
      },
    ],
    requiredIndicators: [
      { type: 'macd', params: ['fast_period', 'slow_period', 'signal_period'] },
    ],
    chartOverlays: [
      { type: 'line', template: 'MACD_DIF' },
      { type: 'line', template: 'MACD_DEA' },
    ],
    uiSchema: { param_order: ['fast_period', 'slow_period', 'signal_period'] },
    executionOptions: {},
    metadata: { aliases: ['MACD趋势策略'] },
  },
  {
    typeKey: 'volume_breakout',
    implementationKey: 'volume_breakout',
    typeLabel: '放量突破',
    shortLabel: '放量突破',
    category: '量价',
    namePrefix: '放量突破策略',
    idPrefix: 'volume-breakout-strategy',
    keyPrefix: 'volume_breakout_strategy',
    defaultDescription: '策略逻辑：当日成交量超过 N 日均量的 M 倍，且收盘价突破 N 日最高价（放量创新高）时买入；收盘价跌破离场均线时卖出。量价齐升是 A 股最经典的启动信号之一。示例：回看=20天、放量倍数=2.0、离场均线=20天，当日量为 20 日均量的 2 倍以上且创 20 日新高触发买入。',
    legacyDescriptions: [],
    paramSchema: [
      {
        key: 'lookback',
        label: '回看周期（天）',
        type: 'integer',
        required: true,
        default: 20,
        min: 5,
        max: 120,
        step: 1,
        description: '计算均量和区间最高价的回看窗口，单位：天。',
        options: [],
      },
      {
        key: 'volume_multiple',
        label: '放量倍数（倍）',
        type: 'number',
        required: true,
        default: 2.0,
        min: 1.2,
        max: 5.0,
        step: 0.1,
        description: '当日成交量 > 均量 × 该倍数时视为放量，单位：倍（例如 2.0 = 2 倍均量）。',
        options: [],
      },
      {
        key: 'exit_ma_period',
        label: '离场均线（天）',
        type: 'integer',
        required: true,
        default: 20,
        min: 5,
        max: 120,
        step: 1,
        description: '收盘价跌破该均线时触发卖出离场，单位：天。',
        options: [],
      },
    ],
    requiredIndicators: [
      { type: 'volume_ma', params: ['lookback'] },
      { type: 'ma', params: ['exit_ma_period'] },
    ],
    chartOverlays: [
      { type: 'line', template: 'MA{exit_ma_period}' },
    ],
    uiSchema: { param_order: ['lookback', 'volume_multiple', 'exit_ma_period'] },
    executionOptions: {},
    metadata: { aliases: ['放量突破策略'] },
  },
  {
    typeKey: 'dual_confirm',
    implementationKey: 'dual_confirm',
    typeLabel: '双重确认（趋势+动量）',
    shortLabel: '双重确认',
    category: '组合',
    namePrefix: '双重确认策略',
    idPrefix: 'dual-confirm-strategy',
    keyPrefix: 'dual_confirm_strategy',
    defaultDescription: '策略逻辑：结合均线交叉（趋势）和 RSI（动量）两个维度进行信号确认。AND 模式下，两个子信号需在确认窗口内先后触发才生效，信号更精准但频率更低；OR 模式下，任一子信号触发即生效，信号更灵敏。示例：AND 模式、MA10/MA30 + RSI14（35/70）+ 窗口5天，均线金叉后 5 天内 RSI 也从 35 下方回升才确认买入。',
    legacyDescriptions: [],
    paramSchema: [
      {
        key: 'ma_short',
        label: '短均线周期（天）',
        type: 'integer',
        required: true,
        default: 10,
        min: 2,
        max: 120,
        step: 1,
        description: '趋势确认：短期均线周期，单位：天。',
        options: [],
      },
      {
        key: 'ma_long',
        label: '长均线周期（天）',
        type: 'integer',
        required: true,
        default: 30,
        min: 5,
        max: 250,
        step: 1,
        description: '趋势确认：长期均线周期，单位：天。',
        options: [],
      },
      {
        key: 'rsi_period',
        label: 'RSI 周期（天）',
        type: 'integer',
        required: true,
        default: 14,
        min: 2,
        max: 60,
        step: 1,
        description: '动量确认：RSI 回看周期，单位：天。',
        options: [],
      },
      {
        key: 'rsi_low',
        label: 'RSI 低位阈值',
        type: 'number',
        required: true,
        default: 35,
        min: 10,
        max: 50,
        step: 1,
        description: 'RSI 上穿该值视为超卖修复，单位：RSI 点（0-100）。',
        options: [],
      },
      {
        key: 'rsi_high',
        label: 'RSI 高位阈值',
        type: 'number',
        required: true,
        default: 70,
        min: 50,
        max: 90,
        step: 1,
        description: 'RSI 下穿该值视为超买回落，单位：RSI 点（0-100）。',
        options: [],
      },
      {
        key: 'confirm_window',
        label: '确认窗口（天）',
        type: 'integer',
        required: true,
        default: 5,
        min: 1,
        max: 20,
        step: 1,
        description: 'AND 模式下，两个子信号需在此窗口内先后触发才算确认，单位：天。',
        options: [],
      },
      {
        key: 'logic_mode',
        label: '组合逻辑',
        type: 'string',
        required: true,
        default: 'and',
        description: 'and = 两个信号都满足才触发；or = 任一满足即触发。',
        options: [
          { value: 'and', label: 'AND（双重确认）' },
          { value: 'or', label: 'OR（任一触发）' },
        ],
      },
    ],
    requiredIndicators: [
      { type: 'ma', params: ['ma_short', 'ma_long'] },
      { type: 'rsi', params: ['rsi_period'] },
    ],
    chartOverlays: [
      { type: 'line', template: 'MA{ma_short}' },
      { type: 'line', template: 'MA{ma_long}' },
      { type: 'line', template: 'RSI_{rsi_period}' },
    ],
    uiSchema: { param_order: ['ma_short', 'ma_long', 'rsi_period', 'rsi_low', 'rsi_high', 'confirm_window', 'logic_mode'] },
    executionOptions: {},
    metadata: { aliases: ['双重确认策略'] },
  },
  {
    typeKey: 'bollinger_macd',
    implementationKey: 'bollinger_macd',
    typeLabel: '布林带+MACD 组合',
    shortLabel: '布林带+MACD',
    category: '组合',
    namePrefix: '布林带MACD策略',
    idPrefix: 'bollinger-macd-strategy',
    keyPrefix: 'bollinger_macd_strategy',
    defaultDescription: '策略逻辑：结合布林带（超买超卖）和 MACD 柱状图（动能方向）两个维度。AND 模式下，价格触及布林带下轨且 MACD 柱状图由负转正（底部共振）才买入，触及上轨且柱状图由正转负（顶部共振）才卖出；OR 模式下任一触发即生效。示例：AND 模式、BB20/2.0 + MACD 12/26/9，价格跌破下轨后 3 天内 MACD 柱翻正确认买入。',
    legacyDescriptions: [],
    paramSchema: [
      {
        key: 'bb_period',
        label: '布林带周期（天）',
        type: 'integer',
        required: true,
        default: 20,
        min: 5,
        max: 100,
        step: 1,
        description: '布林带中轨均线周期，单位：天。',
        options: [],
      },
      {
        key: 'bb_std',
        label: '标准差倍数（倍）',
        type: 'number',
        required: true,
        default: 2.0,
        min: 0.5,
        max: 4.0,
        step: 0.1,
        description: '布林带上下轨宽度，单位：倍。',
        options: [],
      },
      {
        key: 'fast_period',
        label: 'MACD 快线（天）',
        type: 'integer',
        required: true,
        default: 12,
        min: 2,
        max: 50,
        step: 1,
        description: 'MACD EMA 快线周期，单位：天。',
        options: [],
      },
      {
        key: 'slow_period',
        label: 'MACD 慢线（天）',
        type: 'integer',
        required: true,
        default: 26,
        min: 5,
        max: 100,
        step: 1,
        description: 'MACD EMA 慢线周期，单位：天。',
        options: [],
      },
      {
        key: 'signal_period',
        label: 'MACD 信号线（天）',
        type: 'integer',
        required: true,
        default: 9,
        min: 2,
        max: 30,
        step: 1,
        description: 'MACD 信号线（DEA）EMA 平滑周期，单位：天。',
        options: [],
      },
      {
        key: 'logic_mode',
        label: '组合逻辑',
        type: 'string',
        required: true,
        default: 'and',
        description: 'and = 共振确认（更精准）；or = 任一触发（更灵敏）。',
        options: [
          { value: 'and', label: 'AND（共振确认）' },
          { value: 'or', label: 'OR（任一触发）' },
        ],
      },
    ],
    requiredIndicators: [
      { type: 'bollinger', params: ['bb_period', 'bb_std'] },
      { type: 'macd', params: ['fast_period', 'slow_period', 'signal_period'] },
    ],
    chartOverlays: [
      { type: 'line', template: 'BB_upper' },
      { type: 'line', template: 'BB_mid' },
      { type: 'line', template: 'BB_lower' },
      { type: 'line', template: 'MACD_DIF' },
      { type: 'line', template: 'MACD_DEA' },
    ],
    uiSchema: { param_order: ['bb_period', 'bb_std', 'fast_period', 'slow_period', 'signal_period', 'logic_mode'] },
    executionOptions: {},
    metadata: { aliases: ['布林带MACD组合策略'] },
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
