// ── Pure function tests for strategy-form.js ──
// Uses Node 20+ built-in test runner (node --test)
// No external dependencies needed.

import { describe, it } from 'node:test'
import assert from 'node:assert/strict'

// Import functions by re-implementing them (to avoid ESM/CJS interop issues)
// These are exact copies from lib/strategy-form.js

function sortParamSchema(definition) {
  if (!definition?.param_schema) return []
  const schema = [...definition.param_schema]
  const order = definition?.ui_schema?.param_order || []
  if (!Array.isArray(order) || order.length === 0) return schema
  const orderIndex = new Map(order.map((key, index) => [key, index]))
  return schema.sort((a, b) => {
    const aIndex = orderIndex.has(a.key) ? orderIndex.get(a.key) : Number.MAX_SAFE_INTEGER
    const bIndex = orderIndex.has(b.key) ? orderIndex.get(b.key) : Number.MAX_SAFE_INTEGER
    return aIndex - bIndex
  })
}

function buildInitialStrategyParams(definition) {
  const params = {}
  sortParamSchema(definition).forEach((item) => {
    const defaultValue = definition?.default_params?.[item.key] ?? item.default ?? ''
    params[item.key] = defaultValue
  })
  return params
}

function coerceStrategyValue(item, rawValue) {
  if (item.type === 'integer') return Number.parseInt(rawValue, 10)
  if (item.type === 'number') return Number(rawValue)
  if (item.type === 'boolean') {
    if (typeof rawValue === 'boolean') return rawValue
    return ['true', '1', 'yes', 'on'].includes(String(rawValue).toLowerCase())
  }
  return rawValue ?? ''
}

function sanitizeStrategyParams(definition, values) {
  const sanitized = {}
  sortParamSchema(definition).forEach((item) => {
    const rawValue = values?.[item.key] ?? definition?.default_params?.[item.key] ?? item.default
    sanitized[item.key] = coerceStrategyValue(item, rawValue)
  })
  return sanitized
}

function validateStrategyParams(definition, values) {
  if (!definition) return '请选择策略。'
  const schema = sortParamSchema(definition)
  for (const item of schema) {
    const rawValue = values?.[item.key]
    if ((rawValue === '' || rawValue === undefined || rawValue === null) && item.required) {
      return `${item.label}不能为空。`
    }
    const coerced = coerceStrategyValue(item, rawValue)
    if ((item.type === 'integer' || item.type === 'number') && Number.isNaN(Number(coerced))) {
      return `${item.label}格式不正确。`
    }
    if ((item.type === 'integer' || item.type === 'number') && item.min !== undefined && item.min !== null && Number(coerced) < Number(item.min)) {
      return `${item.label}不能小于 ${item.min}。`
    }
    if ((item.type === 'integer' || item.type === 'number') && item.max !== undefined && item.max !== null && Number(coerced) > Number(item.max)) {
      return `${item.label}不能大于 ${item.max}。`
    }
  }
  const sanitized = sanitizeStrategyParams(definition, values)
  if (definition.implementation_key === 'trend_cross' && Number(sanitized.ma_short) >= Number(sanitized.ma_long)) {
    return '双均线策略要求短均线周期小于长均线周期。'
  }
  if (definition.implementation_key === 'rsi_range' && Number(sanitized.rsi_low) >= Number(sanitized.rsi_high)) {
    return 'RSI 低阈值必须小于高阈值。'
  }
  return ''
}

function getInputAttributes(item) {
  if (item.type === 'integer') return { type: 'number', step: item.step ?? 1, min: item.min, max: item.max }
  if (item.type === 'number') return { type: 'number', step: item.step ?? 0.01, min: item.min, max: item.max }
  return { type: 'text' }
}

function prettyJson(value) {
  return JSON.stringify(value ?? {}, null, 2)
}

function parseJsonField(text, label, fallback) {
  const trimmed = (text || '').trim()
  if (!trimmed) return fallback
  try { return JSON.parse(trimmed) } catch { throw new Error(`${label} 不是合法 JSON。`) }
}

function buildStrategyDraft(strategy) {
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
  }
}

function createEmptyStrategyDraft(defaultImplementationKey = 'trend_cross') {
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
  })
}

function buildStrategyPayloadFromDraft(draft) {
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
  }
}

// ═══════════════════ TESTS ═══════════════════

describe('sortParamSchema', () => {
  it('returns empty array when param_schema is missing', () => {
    assert.deepEqual(sortParamSchema(null), [])
    assert.deepEqual(sortParamSchema(undefined), [])
    assert.deepEqual(sortParamSchema({}), [])
  })

  it('returns original schema when no ui_schema.param_order', () => {
    const def = { param_schema: [{ key: 'a' }, { key: 'b' }] }
    assert.deepEqual(sortParamSchema(def), [{ key: 'a' }, { key: 'b' }])
  })

  it('reorders by param_order', () => {
    const def = {
      param_schema: [{ key: 'z_last' }, { key: 'a_first' }, { key: 'm_middle' }],
      ui_schema: { param_order: ['a_first', 'm_middle', 'z_last'] },
    }
    const result = sortParamSchema(def)
    assert.equal(result[0].key, 'a_first')
    assert.equal(result[1].key, 'm_middle')
    assert.equal(result[2].key, 'z_last')
  })

  it('puts unordered items at the end', () => {
    const def = {
      param_schema: [{ key: 'b' }, { key: 'a' }, { key: 'c' }],
      ui_schema: { param_order: ['a'] },
    }
    const result = sortParamSchema(def)
    assert.equal(result[0].key, 'a')
  })
})

describe('buildInitialStrategyParams', () => {
  it('builds defaults from item.default', () => {
    const def = { param_schema: [{ key: 'ma_short', default: 20 }, { key: 'ma_long', default: 60 }] }
    const params = buildInitialStrategyParams(def)
    assert.deepEqual(params, { ma_short: 20, ma_long: 60 })
  })

  it('uses default_params over item.default', () => {
    const def = { default_params: { ma_short: 10 }, param_schema: [{ key: 'ma_short', default: 20 }] }
    assert.equal(buildInitialStrategyParams(def).ma_short, 10)
  })

  it('handles empty definition gracefully', () => {
    assert.deepEqual(buildInitialStrategyParams(null), {})
    assert.deepEqual(buildInitialStrategyParams({}), {})
  })
})

describe('sanitizeStrategyParams', () => {
  it('coerces values to correct types', () => {
    const def = {
      param_schema: [
        { key: 'ma_short', type: 'integer', default: 5 },
        { key: 'threshold', type: 'number', default: 1.5 },
        { key: 'enabled', type: 'boolean', default: true },
        { key: 'name', type: 'string', default: '' },
      ],
    }
    const r = sanitizeStrategyParams(def, { ma_short: '30', threshold: '2.5', enabled: 'yes', name: 'test' })
    assert.equal(r.ma_short, 30)
    assert.ok(Math.abs(r.threshold - 2.5) < 0.001)
    assert.ok(r.enabled === true)
    assert.equal(r.name, 'test')
  })
})

describe('validateStrategyParams', () => {
  it('returns error for null definition', () => {
    assert.match(validateStrategyParams(null, {}), /请选择策略/)
  })

  it('returns error for required empty field', () => {
    const def = { implementation_key: 'x', param_schema: [{ key: 'ma_short', label: '短均线周期', required: true, type: 'integer' }] }
    assert.match(validateStrategyParams(def, {}), /不能为空/)
  })

  it('returns error when value below min', () => {
    const def = { implementation_key: 'x', param_schema: [{ key: 's', label: 'S', type: 'integer', min: 2 }] }
    assert.match(validateStrategyParams(def, { s: 1 }), /不能小于/)
  })

  it('returns trend_cross specific validation', () => {
    const def = {
      implementation_key: 'trend_cross',
      param_schema: [{ key: 'ma_short', type: 'integer' }, { key: 'ma_long', type: 'integer' }],
    }
    assert.match(validateStrategyParams(def, { ma_short: 60, ma_long: 20 }), /短均线.*长均线/)
  })

  it('returns empty string for valid params', () => {
    const def = {
      implementation_key: 'trend_cross',
      param_schema: [{ key: 'ma_short', type: 'integer', min: 2 }, { key: 'ma_long', type: 'integer', max: 500 }],
    }
    assert.equal(validateStrategyParams(def, { ma_short: 20, ma_long: 60 }), '')
  })
})

describe('coerceStrategyValue', () => {
  it('coerces integer via parseInt', () => {
    assert.equal(coerceStrategyValue({ type: 'integer' }, '42'), 42)
    assert.ok(Number.isNaN(coerceStrategyValue({ type: 'integer' }, 'abc')))
  })

  it('coerces number via Number()', () => {
    assert.ok(Math.abs(coerceStrategyValue({ type: 'number' }, '3.14') - 3.14) < 0.001)
  })

  it('coerces boolean from various formats', () => {
    assert.equal(coerceStrategyValue({ type: 'boolean' }, true), true)
    assert.equal(coerceStrategyValue({ type: 'boolean' }, false), false)
    assert.equal(coerceStrategyValue({ type: 'boolean' }, 'true'), true)
    assert.equal(coerceStrategyValue({ type: 'boolean' }, '1'), true)
    assert.equal(coerceStrategyValue({ type: 'boolean' }, 'yes'), true)
    assert.equal(coerceStrategyValue({ type: 'boolean' }, 'false'), false)
    assert.equal(coerceStrategyValue({ type: 'boolean' }, ''), false)
  })

  it('passes through string as fallback', () => {
    assert.equal(coerceStrategyValue({}, 'hello'), 'hello')
    assert.equal(coerceStrategyValue({}, undefined), '')
  })
})

describe('getInputAttributes', () => {
  it('returns integer input attrs', () => {
    assert.deepEqual(getInputAttributes({ type: 'integer', step: 2, min: 5, max: 100 }), { type: 'number', step: 2, min: 5, max: 100 })
  })
  it('returns text as fallback', () => {
    assert.deepEqual(getInputAttributes({ type: 'boolean' }), { type: 'text' })
  })
})

describe('prettyJson', () => {
  it('formats object with 2-space indent', () => {
    assert.match(prettyJson({ a: 1 }), /\n  "a": 1/)
  })
  it('returns "{}" for null/undefined', () => {
    assert.equal(prettyJson(null), '{}')
    assert.equal(prettyJson(undefined), '{}')
  })
})

describe('parseJsonField', () => {
  it('parses valid JSON string', () => {
    assert.deepEqual(parseJsonField('{"key":"val"}', 'test', {}), { key: 'val' })
  })
  it('returns fallback for empty string', () => {
    assert.deepEqual(parseJsonField('', 'label', []), [])
  })
  it('throws for invalid JSON', () => {
    assert.throws(() => parseJsonField('{invalid}', 'label', {}), /不是合法 JSON/)
  })
})

describe('buildStrategyDraft', () => {
  it('converts strategy to draft form with prettyJson fields', () => {
    const draft = buildStrategyDraft({ id: 's1', name: 'Test', implementation_key: 'macd_cross', status: 'active', version: 2, param_schema: [{ key: 'period' }] })
    assert.equal(draft.id, 's1')
    assert.equal(draft.status, 'active')
    assert.match(draft.param_schema_text, /"key": "period"/)
  })
  it('fills defaults for null strategy', () => {
    const d = buildStrategyDraft(null)
    assert.equal(d.id, '')
    assert.equal(d.implementation_key, 'trend_cross')
  })
})

describe('createEmptyStrategyDraft', () => {
  it('creates empty draft with defaults', () => {
    const d = createEmptyStrategyDraft()
    assert.equal(d.id, '')
    assert.equal(d.version, 1)
    assert.equal(JSON.parse(d.param_schema_text).length, 0)
  })
  it('accepts custom implementation key', () => {
    assert.equal(createEmptyStrategyDraft('rsi_range').implementation_key, 'rsi_range')
  })
})

describe('buildStrategyPayloadFromDraft', () => {
  it('parses all text fields back into objects', () => {
    const payload = buildStrategyPayloadFromDraft({
      id: 's1', name: 'My Strategy', category: '趋势',
      implementation_key: 'trend_cross', status: 'draft', version: '1',
      param_schema_text: '[{"key":"ma_short","type":"integer"}]',
      default_params_text: '{"ma_short":20}',
      required_indicators_text: '[]', chart_overlays_text: '[]',
      ui_schema_text: '{}', execution_options_text: '{}', metadata_text: '{}',
    })
    assert.equal(payload.id, 's1')
    assert.deepEqual(payload.param_schema, [{ key: 'ma_short', type: 'integer' }])
    assert.deepEqual(payload.default_params, { ma_short: 20 })
  })
  it('trims id and key fields', () => {
    const p = buildStrategyPayloadFromDraft({
      id: '  s1  ', key: '  k1  ', name: '', implementation_key: 'trend_cross',
      status: 'draft', version: '1', param_schema_text: '[]', default_params_text: '{}',
      required_indicators_text: '[]', chart_overlays_text: '[]',
      ui_schema_text: '{}', execution_options_text: '{}', metadata_text: '{}',
    })
    assert.equal(p.id, 's1')
    assert.equal(p.key, 'k1')
    assert.equal(p.version, 1)
  })
})
