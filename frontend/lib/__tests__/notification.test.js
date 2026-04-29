import { describe, it, beforeEach } from 'node:test'
import assert from 'node:assert/strict'

import {
  buildAIAnalysisNotification,
  buildNotificationPayload,
  isIOSSafari,
  isNotificationSupported,
  getNotificationPermission,
  loadNotificationPreferences,
  saveNotificationPreferences,
  isCategoryEnabled,
  NOTIFICATION_CATEGORIES,
} from '../notification.js'

describe('buildAIAnalysisNotification', () => {
  it('returns correct payload for buy signal with high confidence', () => {
    const payload = buildAIAnalysisNotification({
      symbol: '600519.SH',
      symbolName: '贵州茅台',
      signal: 'buy',
      confidenceScore: 78,
    })
    assert.equal(payload.title, 'AI 分析完成 · 贵州茅台')
    assert.ok(payload.options.body.includes('看多'))
    assert.ok(payload.options.body.includes('高置信度'))
    assert.equal(payload.options.tag, 'ai-analysis-600519.SH')
    assert.equal(payload.options.icon, '/favicon.ico')
    assert.equal(payload.options.silent, true)
    assert.equal(payload.options.requireInteraction, false)
  })

  it('returns correct payload for sell signal with medium confidence', () => {
    const payload = buildAIAnalysisNotification({
      symbol: '00700.HK',
      symbolName: '腾讯控股',
      signal: 'sell',
      confidenceScore: 55,
    })
    assert.equal(payload.title, 'AI 分析完成 · 腾讯控股')
    assert.ok(payload.options.body.includes('看空'))
    assert.ok(payload.options.body.includes('中等置信度'))
    assert.equal(payload.options.tag, 'ai-analysis-00700.HK')
  })

  it('returns correct payload for hold signal with low confidence', () => {
    const payload = buildAIAnalysisNotification({
      symbol: '000001.SZ',
      symbolName: '平安银行',
      signal: 'hold',
      confidenceScore: 30,
    })
    assert.equal(payload.title, 'AI 分析完成 · 平安银行')
    assert.ok(payload.options.body.includes('观望'))
    assert.ok(payload.options.body.includes('低置信度'))
  })

  it('falls back to symbol when symbolName is missing', () => {
    const payload = buildAIAnalysisNotification({
      symbol: '000001.SZ',
      signal: 'buy',
    })
    assert.equal(payload.title, 'AI 分析完成 · 000001.SZ')
  })

  it('falls back to hold for unknown signal', () => {
    const payload = buildAIAnalysisNotification({
      symbol: 'TEST',
      signal: 'unknown',
    })
    assert.ok(payload.options.body.includes('观望'))
  })

  it('clamps confidence score to 0~100', () => {
    const high = buildAIAnalysisNotification({ symbol: 'A', signal: 'buy', confidenceScore: 150 })
    assert.ok(high.options.body.includes('高置信度'))
    const low = buildAIAnalysisNotification({ symbol: 'B', signal: 'buy', confidenceScore: -10 })
    assert.ok(low.options.body.includes('低置信度'))
  })
})

describe('buildNotificationPayload', () => {
  it('delegates to buildAIAnalysisNotification for ai_analysis category', () => {
    const payload = buildNotificationPayload(NOTIFICATION_CATEGORIES.AI_ANALYSIS, {
      symbol: '600519.SH',
      symbolName: '贵州茅台',
      signal: 'buy',
      confidenceScore: 80,
    })
    assert.ok(payload)
    assert.equal(payload.title, 'AI 分析完成 · 贵州茅台')
  })

  it('returns null for unknown category', () => {
    const payload = buildNotificationPayload('unknown_category', {})
    assert.equal(payload, null)
  })
})

describe('isIOSSafari', () => {
  const originalUA = global.navigator?.userAgent

  beforeEach(() => {
    delete global.navigator
    delete global.window
  })

  it('returns true for iPhone Safari', () => {
    global.window = { MSStream: undefined }
    global.navigator = { userAgent: 'Mozilla/5.0 (iPhone; CPU iPhone OS 16_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/16.0 Mobile/15E148 Safari/604.1' }
    assert.equal(isIOSSafari(), true)
  })

  it('returns true for iPad Safari', () => {
    global.window = { MSStream: undefined }
    global.navigator = { userAgent: 'Mozilla/5.0 (iPad; CPU OS 16_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/16.0 Mobile/15E148 Safari/604.1' }
    assert.equal(isIOSSafari(), true)
  })

  it('returns false for Android Chrome', () => {
    global.window = { MSStream: undefined }
    global.navigator = { userAgent: 'Mozilla/5.0 (Linux; Android 13; SM-S918B) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/112.0.0.0 Mobile Safari/537.36' }
    assert.equal(isIOSSafari(), false)
  })

  it('returns false for desktop Chrome', () => {
    global.window = { MSStream: undefined }
    global.navigator = { userAgent: 'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/112.0.0.0 Safari/537.36' }
    assert.equal(isIOSSafari(), false)
  })

  it('returns false when window is undefined', () => {
    delete global.window
    global.navigator = { userAgent: 'iPhone Safari' }
    assert.equal(isIOSSafari(), false)
  })
})

describe('isNotificationSupported', () => {
  it('returns false when window.Notification is absent', () => {
    global.window = {}
    assert.equal(isNotificationSupported(), false)
  })

  it('returns true when window.Notification exists', () => {
    global.window = { Notification: class FakeNotification {} }
    assert.equal(isNotificationSupported(), true)
  })
})

describe('getNotificationPermission', () => {
  it('returns unsupported when Notification API is absent', () => {
    global.window = {}
    assert.equal(getNotificationPermission(), 'unsupported')
  })

  it('returns current permission when supported', () => {
    global.Notification = { permission: 'granted' }
    global.window = { Notification: global.Notification }
    assert.equal(getNotificationPermission(), 'granted')
  })
})

describe('loadNotificationPreferences', () => {
  beforeEach(() => {
    delete global.window
  })

  it('returns defaults when window is undefined', () => {
    const prefs = loadNotificationPreferences()
    assert.equal(prefs.aiAnalysis, true)
    assert.equal(prefs.signalTrigger, false)
    assert.equal(prefs.system, true)
  })

  it('returns defaults when localStorage is empty', () => {
    global.window = { localStorage: { getItem: () => null } }
    const prefs = loadNotificationPreferences()
    assert.equal(prefs.aiAnalysis, true)
    assert.equal(prefs.signalTrigger, false)
    assert.equal(prefs.system, true)
  })

  it('parses saved preferences correctly', () => {
    global.window = {
      localStorage: {
        getItem: () => JSON.stringify({ aiAnalysis: false, signalTrigger: true, system: false }),
      },
    }
    const prefs = loadNotificationPreferences()
    assert.equal(prefs.aiAnalysis, false)
    assert.equal(prefs.signalTrigger, true)
    assert.equal(prefs.system, false)
  })

  it('falls back to defaults on parse error', () => {
    global.window = {
      localStorage: {
        getItem: () => 'invalid-json',
      },
    }
    const prefs = loadNotificationPreferences()
    assert.equal(prefs.aiAnalysis, true)
    assert.equal(prefs.signalTrigger, false)
    assert.equal(prefs.system, true)
  })
})

describe('saveNotificationPreferences', () => {
  beforeEach(() => {
    delete global.window
  })

  it('returns false when window is undefined', () => {
    const result = saveNotificationPreferences({ aiAnalysis: false })
    assert.equal(result, false)
  })

  it('saves normalized preferences to localStorage', () => {
    const store = {}
    global.window = {
      localStorage: {
        setItem: (key, value) => { store[key] = value },
      },
    }
    const result = saveNotificationPreferences({ aiAnalysis: false, signalTrigger: true })
    assert.equal(result, true)
    assert.ok(store['wolong_notification_prefs'])
    const parsed = JSON.parse(store['wolong_notification_prefs'])
    assert.equal(parsed.aiAnalysis, false)
    assert.equal(parsed.signalTrigger, true)
    assert.equal(parsed.system, true)
  })
})

describe('isCategoryEnabled', () => {
  beforeEach(() => {
    delete global.window
  })

  it('returns true for ai_analysis by default', () => {
    global.window = { localStorage: { getItem: () => null } }
    assert.equal(isCategoryEnabled(NOTIFICATION_CATEGORIES.AI_ANALYSIS), true)
  })

  it('returns false when ai_analysis is disabled', () => {
    global.window = {
      localStorage: {
        getItem: () => JSON.stringify({ aiAnalysis: false }),
      },
    }
    assert.equal(isCategoryEnabled(NOTIFICATION_CATEGORIES.AI_ANALYSIS), false)
  })

  it('returns false for signal_trigger by default', () => {
    global.window = { localStorage: { getItem: () => null } }
    assert.equal(isCategoryEnabled(NOTIFICATION_CATEGORIES.SIGNAL_TRIGGER), false)
  })

  it('returns true for system by default', () => {
    global.window = { localStorage: { getItem: () => null } }
    assert.equal(isCategoryEnabled(NOTIFICATION_CATEGORIES.SYSTEM), true)
  })

  it('returns false for unknown category', () => {
    global.window = { localStorage: { getItem: () => null } }
    assert.equal(isCategoryEnabled('unknown'), false)
  })
})
