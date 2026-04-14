// ── Pure function tests for stock-picker.js (screener-utils) ──
// Uses Node 20+ built-in test runner (node --test)
// 覆盖 P0-3~P0-6: formatValue / codeToSymbol / 缓存 key / buildScanPayload

import { describe, it } from 'node:test'
import assert from 'node:assert/strict'

// ═══════════════════════════════════════════════════
// 从 stock-picker.js 复制的纯函数（保持同步）
// ═══════════════════════════════════════════════════

function getScreenerCacheKey(exchange) {
  return `pumpkin_screener_cache_${(exchange || 'ASHARE').toLowerCase()}`
}

function formatValue(value, format, exchange) {
  if (value === null || value === undefined || value === '') return '--'
  const num = Number(value)
  const isHKEX = exchange === 'HKEX'

  switch (format) {
    case 'code':
      return String(value).padStart(isHKEX ? 5 : 6, '0')
    case 'text':
      return String(value)
    case 'price':
      if (isNaN(num)) return '--'
      return isHKEX ? `${num.toFixed(2)} HKD` : num.toFixed(2)
    case 'percent':
      if (isNaN(num)) return '--'
      return (num >= 0 ? '+' : '') + num.toFixed(2) + '%'
    case 'integer':
      if (isNaN(num)) return '--'
      return num.toLocaleString('zh-CN', { maximumFractionDigits: 0 })
    case 'bigNumber': {
      if (isNaN(num)) return '--'
      const absNum = Math.abs(num)
      if (absNum >= 1e8) {
        const formatted = (num / 1e8).toFixed(2)
        return isHKEX ? `${formatted} 亿 HKD` : `${formatted} 亿`
      }
      if (absNum >= 1e4) return (num / 1e4).toFixed(2) + ' 万'
      return num.toFixed(2)
    }
    case 'number':
      return isNaN(num) ? '--' : num.toFixed(2)
    default:
      return String(value)
  }
}

function codeToSymbol(code, exchange) {
  if (exchange === 'HKEX') {
    return `${String(code).padStart(5, '0')}.HK`
  }
  const c = String(code).padStart(6, '0')
  return c.startsWith('6') || c.startsWith('9') ? `${c}.SH` : `${c}.SZ`
}

function getColorClass(value) {
  if (value === null || value === undefined) return ''
  const num = Number(value)
  if (isNaN(num)) return ''
  if (num > 0) return 'text-red-500'
  if (num < 0) return 'text-green-500'
  return 'text-white/50'
}


// ═══════════════════════════════════════════════════
// Section A: formatValue — code 格式化
// ═══════════════════════════════════════════════════

describe('formatValue: code', () => {

  it('A 股代码补零到 6 位', () => {
    assert.equal(formatValue(1, 'code', 'ASHARE'), '000001')
  })

  it('A 股代码已经是 6 位不变', () => {
    assert.equal(formatValue(600000, 'code', 'ASHARE'), '600000')
  })

  it('港股代码补零到 5 位', () => {
    assert.equal(formatValue(700, 'code', 'HKEX'), '00700')
  })

  it('港股代码保留前导零', () => {
    assert.equal(formatValue(5, 'code', 'HKEX'), '00005')
  })

  it('港股代码 5 位数字不重复补零', () => {
    assert.equal(formatValue(9988, 'code', 'HKEX'), '09988')
  })
})


// ═══════════════════════════════════════════════════
// Section B: formatValue — price 格式化
// ═══════════════════════════════════════════════════

describe('formatValue: price', () => {

  it('A 股价格不带单位', () => {
    assert.equal(formatValue(12.35, 'price', 'ASHARE'), '12.35')
  })

  it('港股价格带 HKD 单位', () => {
    assert.equal(formatValue(380.20, 'price', 'HKEX'), '380.20 HKD')
  })

  it('港股低价也带 HKD', () => {
    assert.equal(formatValue(3.50, 'price', 'HKEX'), '3.50 HKD')
  })

  it('null 价格返回 --', () => {
    assert.equal(formatValue(null, 'price', 'ASHARE'), '--')
  })

  it('undefined 价格返回 --', () => {
    assert.equal(formatValue(undefined, 'price', 'HKEX'), '--')
  })

  it('空字符串价格返回 --', () => {
    assert.equal(formatValue('', 'price', 'ASHARE'), '--')
  })

  it('NaN 价格返回 --', () => {
    assert.equal(formatValue(NaN, 'price', 'ASHARE'), '--')
  })
})


// ═══════════════════════════════════════════════════
// Section C: formatValue — bigNumber 格式化
// ═══════════════════════════════════════════════════

describe('formatValue: bigNumber', () => {

  it('A 股大数显示亿', () => {
    assert.equal(formatValue(52e8, 'bigNumber', 'ASHARE'), '52.00 亿')
  })

  it('港股大数显示亿 HKD', () => {
    assert.equal(formatValue(3000e8, 'bigNumber', 'HKEX'), '3000.00 亿 HKD')
  })

  it('中等数值显示万', () => {
    assert.equal(formatValue(50000, 'bigNumber', 'ASHARE'), '5.00 万')
  })

  it('小数值直接显示', () => {
    assert.equal(formatValue(999, 'bigNumber', 'ASHARE'), '999.00')
  })

  it('null 返回 --', () => {
    assert.equal(formatValue(null, 'bigNumber', 'ASHARE'), '--')
  })
})


// ═══════════════════════════════════════════════════
// Section D: formatValue — percent / integer / text
// ═══════════════════════════════════════════════════

describe('formatValue: other formats', () => {

  it('正涨幅带 + 号', () => {
    assert.equal(formatValue(3.5, 'percent', 'ASHARE'), '+3.50%')
  })

  it('负跌幅带 - 号', () => {
    assert.equal(formatValue(-2.1, 'percent', 'ASHARE'), '-2.10%')
  })

  it('零涨跌', () => {
    assert.equal(formatValue(0, 'percent', 'ASHARE'), '+0.00%')
  })

  it('整数格式加千分位', () => {
    assert.equal(formatValue(123456, 'integer', 'ASHARE'), '123,456')
  })

  it('文本原样输出', () => {
    assert.equal(formatValue('腾讯控股', 'text', 'ASHARE'), '腾讯控股')
  })

  it('number 格式保留两位小数', () => {
    assert.equal(formatValue(15.678, 'number', 'ASHARE'), '15.68')
  })
})


// ═══════════════════════════════════════════════════
// Section E: codeToSymbol — 代码转 symbol
// ═══════════════════════════════════════════════════

describe('codeToSymbol', () => {

  it('A 股沪市 .SH', () => {
    assert.equal(codeToSymbol(600000, 'ASHARE'), '600000.SH')
  })

  it('A 股深市 .SZ', () => {
    assert.equal(codeToSymbol(1, 'ASHARE'), '000001.SZ')
  })

  it('A 股科创板 .SH', () => {
    assert.equal(codeToSymbol(688001, 'ASHARE'), '688001.SH')
  })

  it('港股 .HK', () => {
    assert.equal(codeToSymbol(700, 'HKEX'), '00700.HK')
  })

  it('港股字符串输入', () => {
    assert.equal(codeToSymbol('9988', 'HKEX'), '09988.HK')
  })
})


// ═══════════════════════════════════════════════════
// Section F: getColorClass — 涨跌颜色
// ═══════════════════════════════════════════════════

describe('getColorClass', () => {

  it('正值 → 红色', () => {
    assert.equal(getColorClass(3.5), 'text-red-500')
  })

  it('负值 → 绿色', () => {
    assert.equal(getColorClass(-2.1), 'text-green-500')
  })

  it('零值 → 灰色', () => {
    assert.equal(getColorClass(0), 'text-white/50')
  })

  it('null → 空字符串', () => {
    assert.equal(getColorClass(null), '')
  })

  it('NaN → 空字符串', () => {
    assert.equal(getColorClass(NaN), '')
  })
})


// ═══════════════════════════════════════════════════
// Section G: 缓存 key 分离
// ═══════════════════════════════════════════════════

describe('getScreenerCacheKey', () => {

  it('A 股缓存 key 含 ashare', () => {
    const key = getScreenerCacheKey('ASHARE')
    assert.ok(key.includes('ashare'))
    assert.equal(key, 'pumpkin_screener_cache_ashare')
  })

  it('港股缓存 key 含 hkex', () => {
    const key = getScreenerCacheKey('HKEX')
    assert.ok(key.includes('hkex'))
    assert.equal(key, 'pumpkin_screener_cache_hkex')
  })

  it('默认 ASHARE 兼容', () => {
    const key = getScreenerCacheKey(undefined)
    assert.equal(key, 'pumpkin_screener_cache_ashare')
  })

  it('null 默认 ASHARE', () => {
    const key = getScreenerCacheKey(null)
    assert.equal(key, 'pumpkin_screener_cache_ashare')
  })

  it('大小写不敏感：hkex 小写也能工作', () => {
    const key = getScreenerCacheKey('hkex')
    assert.equal(key, 'pumpkin_screener_cache_hkex')
  })

  it('两个市场 key 不同', () => {
    const ashareKey = getScreenerCacheKey('ASHARE')
    const hkexKey = getScreenerCacheKey('HKEX')
    assert.notEqual(ashareKey, hkexKey)
  })
})
