import {
  STRATEGY_PRESETS,
  getStrategyPresetByType,
  getStrategyPresetByImplementation,
  buildPresetDefinition,
  buildDraftFromStrategy,
  createDraftFromType,
  buildPayloadFromDraft,
  resolveStrategyDescription,
} from '../strategy-presets.js';
import { describe, it } from 'node:test';
import assert from 'node:assert/strict';

// ── STRATEGY_PRESETS ──

describe('STRATEGY_PRESETS', () => {
  it('should have exactly 8 presets', () => {
    assert.equal(STRATEGY_PRESETS.length, 8);
  });

  it('each preset should have required fields', () => {
    for (const p of STRATEGY_PRESETS) {
      assert.ok(p.typeKey, 'missing typeKey');
      assert.ok(p.implementationKey, 'missing implementationKey');
      assert.ok(p.typeLabel, 'missing typeLabel');
      assert.ok(p.paramSchema, 'missing paramSchema');
      assert.ok(Array.isArray(p.requiredIndicators));
      assert.ok(Array.isArray(p.chartOverlays));
    }
  });

  it('all typeKeys should be unique', () => {
    const keys = STRATEGY_PRESETS.map((p) => p.typeKey);
    assert.equal(new Set(keys).size, keys.length);
  });

  it('all implementationKeys should be unique', () => {
    const keys = STRATEGY_PRESETS.map((p) => p.implementationKey);
    assert.equal(new Set(keys).size, keys.length);
  });
});

// ── getStrategyPresetByType() ──

describe('getStrategyPresetByType()', () => {
  it('returns preset for known typeKey', () => {
    const preset = getStrategyPresetByType('macd_cross');
    assert.ok(preset);
    assert.equal(preset.typeKey, 'macd_cross');
    assert.equal(preset.shortLabel, 'MACD');
  });

  it('returns null for unknown typeKey', () => {
    assert.equal(getStrategyPresetByType('nonexistent'), null);
  });

  it('can find all 8 presets by typeKey', () => {
    const found = STRATEGY_PRESETS
      .map((p) => getStrategyPresetByType(p.typeKey))
      .filter(Boolean);
    assert.equal(found.length, 8);
  });
});

// ── getStrategyPresetByImplementation() ──

describe('getStrategyPresetByImplementation()', () => {
  it('returns preset for known implementationKey', () => {
    const preset = getStrategyPresetByImplementation('grid');
    assert.ok(preset);
    assert.equal(preset.implementationKey, 'grid');
    assert.equal(preset.category, '震荡');
  });

  it('returns null for unknown key', () => {
    assert.equal(getStrategyPresetByImplementation('nope'), null);
  });

  it('can find all 8 presets by implementationKey', () => {
    const found = STRATEGY_PRESETS
      .map((p) => getStrategyPresetByImplementation(p.implementationKey))
      .filter(Boolean);
    assert.equal(found.length, 8);
  });
});

// ── buildPresetDefinition() ──

describe('buildPresetDefinition()', () => {
  it('returns null for null input', () => {
    assert.equal(buildPresetDefinition(null), null);
  });

  it('builds definition with correct shape for macd_cross', () => {
    const preset = getStrategyPresetByType('macd_cross');
    const def = buildPresetDefinition(preset);
    assert.ok(def);
    assert.equal(def.implementation_key, 'macd_cross');
    assert.ok(Array.isArray(def.param_schema));
    assert.ok(typeof def.default_params === 'object');
    assert.ok(def.ui_schema);
  });

  it('default_params has values from schema defaults', () => {
    const preset = getStrategyPresetByType('trend_cross');
    const def = buildPresetDefinition(preset);
    assert.equal(def.default_params.ma_short, 20);
    assert.equal(def.default_params.ma_long, 60);
  });
});

// ── resolveStrategyDescription() ──

describe('resolveStrategyDescription()', () => {
  it('returns defaultDescription for empty description', () => {
    const preset = getStrategyPresetByType('macd_cross');
    const result = resolveStrategyDescription('', preset);
    assert.equal(result, preset.defaultDescription);
  });

  it('returns defaultDescription for null description', () => {
    const preset = getStrategyPresetByType('macd_cross');
    const result = resolveStrategyDescription(null, preset);
    assert.equal(result, preset.defaultDescription);
  });

  it('returns custom non-legacy description as-is', () => {
    const preset = getStrategyPresetByType('trend_cross');
    const custom = '我的自定义策略描述';
    assert.equal(resolveStrategyDescription(custom, preset), custom);
  });

  it('replaces legacy description with defaultDescription', () => {
    const preset = getStrategyPresetByType('trend_cross');
    const legacy = preset.legacyDescriptions[0];
    assert.equal(resolveStrategyDescription(legacy, preset), preset.defaultDescription);
  });

  it('returns empty string when no preset provided', () => {
    assert.equal(resolveStrategyDescription('', null), '');
    assert.equal(resolveStrategyDescription('hello', null), 'hello');
  });
});

// ── createDraftFromType() ──

describe('createDraftFromType()', () => {
  it('creates draft with correct shape for trend_cross', () => {
    const draft = createDraftFromType('trend_cross', []);
    assert.ok(draft.id.startsWith('trend-strategy-'));
    assert.ok(draft.key.startsWith('trend_strategy_'));
    assert.ok(draft.name.includes('趋势跟踪策略'));
    assert.equal(draft.status, 'draft');
    assert.equal(draft.version, 1);
    assert.equal(draft.typeKey, 'trend_cross');
    assert.ok(draft.params);
  });

  it('uses next available index avoiding conflicts', () => {
    const existing = [
      { id: 'grid-strategy-1', key: 'grid_strategy_1', name: '网格交易策略 1' },
      { id: 'grid-strategy-2', key: 'grid_strategy_2', name: '网格交易策略 2' },
    ];
    const draft = createDraftFromType('grid', existing);
    assert.equal(draft.id, 'grid-strategy-3');
    assert.equal(draft.key, 'grid_strategy_3');
  });

  it('throws error for unknown typeKey', () => {
    assert.throws(() => createDraftFromType('unknown'), /未识别的策略类型/);
  });

  it('draft params contain defaults from schema', () => {
    const draft = createDraftFromType('rsi_range', []);
    assert.equal(draft.params.rsi_period, 14);
    assert.equal(draft.params.rsi_low, 30);
    assert.equal(draft.params.rsi_high, 70);
  });
});

// ── buildPayloadFromDraft() ──

describe('buildPayloadFromDraft()', () => {
  it('builds payload from minimal draft', () => {
    const payload = buildPayloadFromDraft({
      id: 'test-1',
      key: 'test_k',
      name: 'Test Strategy',
      typeKey: 'bollinger_reversion',
      status: 'active',
      version: 2,
    });
    assert.equal(payload.id, 'test-1');
    assert.equal(payload.key, 'test_k');
    assert.equal(payload.implementation_key, 'bollinger_reversion');
    assert.equal(payload.status, 'active');
    assert.equal(payload.version, 2);
    assert.ok(payload.param_schema);
    assert.ok(payload.default_params);
    assert.ok(payload.required_indicators);
  });

  it('uses custom description when provided', () => {
    const payload = buildPayloadFromDraft({
      typeKey: 'macd_cross',
      description: 'Custom MACD strategy',
    });
    assert.equal(payload.description, 'Custom MACD strategy');
  });

  it('falls back to default description when empty', () => {
    const payload = buildPayloadFromDraft({
      typeKey: 'macd_cross',
      description: '',
    });
    assert.ok(payload.description.length > 50); // default is long text
  });

  it('throws for invalid typeKey', () => {
    assert.throws(
      () => buildPayloadFromDraft({ typeKey: 'invalid' }),
      /未选择有效的策略类型/
    );
  });

  it('trims id and key fields', () => {
    const payload = buildPayloadFromDraft({
      id: '  spaced-id  ',
      key: '  spaced-key  ',
      typeKey: 'grid',
    });
    assert.equal(payload.id, 'spaced-id');
    assert.equal(payload.key, 'spaced-key');
  });
});
