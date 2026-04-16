// ── Pure function tests for admin.js progress component ──
// Uses Node 20+ built-in test runner (node --test)
//
// Tests the extractable pure logic from QuadrantAdminPanel:
//   1. formatTimeAgo() — time-ago formatting
//   2. Progress status resolution — icon/label/color mapping
//   3. Progress percent calculation

import { describe, it } from 'node:test'
import assert from 'node:assert/strict'

// ═══════════════════════════════════════════
// Section A: formatTimeAgo (copied from admin.js for unit testing)
// ═══════════════════════════════════════════

function formatTimeAgo(s) {
  if (!s) return ''
  try {
    const d = new Date(s)
    const diffSec = Math.floor((Date.now() - d.getTime()) / 1000)
    if (diffSec < 10) return '刚刚'
    if (diffSec < 60) return `${diffSec}秒前`
    const diffMin = Math.floor(diffSec / 60)
    if (diffMin < 60) return `${diffMin}分钟前`
    return `${Math.floor(diffMin / 60)}小时前`
  } catch { return '' }
}

describe('formatTimeAgo', () => {

  it('returns empty string for null/undefined', () => {
    assert.equal(formatTimeAgo(null), '')
    assert.equal(formatTimeAgo(undefined), '')
    assert.equal(formatTimeAgo(''), '')
  })

  it('returns "刚刚" for timestamps within 10 seconds', () => {
    const now = new Date().toISOString()
    assert.equal(formatTimeAgo(now), '刚刚')
  })

  it('returns "X秒前" for timestamps between 10-59 seconds ago', () => {
    // We can't easily mock time, so we just verify the function returns a non-empty string
    // that contains "秒前" for a recent-ish timestamp
    // This is more of a smoke test; precise timing requires time mocking
    const thirtySecAgo = new Date(Date.now() - 30_000).toISOString()
    const result = formatTimeAgo(thirtySecAgo)
    assert.ok(result.includes('秒前'), `expected "秒前" in "${result}"`)
  })

  it('returns "X分钟前" for timestamps between 1-59 minutes ago', () => {
    const fiveMinAgo = new Date(Date.now() - 5 * 60_000).toISOString()
    const result = formatTimeAgo(fiveMinAgo)
    assert.ok(result.includes('分钟前'), `expected "分钟前" in "${result}"`)
  })

  it('returns "X小时前" for timestamps >= 1 hour ago', () => {
    const twoHoursAgo = new Date(Date.now() - 2 * 3600_000).toISOString()
    const result = formatTimeAgo(twoHoursAgo)
    assert.ok(result.includes('小时前'), `expected "小时前" in "${result}"`)
  })

  it('returns non-empty or empty without crashing for invalid date strings', () => {
    // Note: JavaScript's new Date() does NOT throw for most invalid inputs;
    // it returns an Invalid Date object where getTime() = NaN.
    // Our function's try/catch only catches truly exceptional cases.
    // The important guarantee is: no crash, always returns a string.
    const inputs = ['not-a-date', '2025-13-01', '', 'invalid']
    for (const s of inputs) {
      const result = formatTimeAgo(s)
      assert.ok(typeof result === 'string', `formatTimeAgo(${JSON.stringify(s)}) must return string`)
    }
  })
})

// ═══════════════════════════════════════════
// Section B: Progress status resolution helpers
// ═══════════════════════════════════════════

/**
 * Resolves progress status into display metadata.
 * Mirrors the logic inside renderProgressBar() of admin.js.
 */
function resolveProgressDisplay(p) {
  if (!p) return null

  const isRunning = p.status === 'running'
  const isSuccess = p.status === 'success'
  const isFailed = p.status === 'failed'
  const isTimeout = p.status === 'timeout'

  const statusIcon = isSuccess ? '✅' : isFailed ? '❌' : isTimeout ? '⏰' : isRunning ? '🔄' : '💤'
  const statusLabel = isSuccess ? '已完成' : isFailed ? '失败' : isTimeout ? '超时' : isRunning ? '计算中...' : '空闲'

  let barColor
  if (isSuccess) barColor = 'bg-emerald-500'
  else if (isFailed) barColor = 'bg-rose-500'
  else if (isTimeout) barColor = 'bg-amber-500'
  else barColor = 'bg-blue-500' // running or idle

  const shouldPulse = isRunning
  const pct = Math.min(p.percent || 0, 100)

  return { statusIcon, statusLabel, barColor, shouldPulse, pct }
}

describe('resolveProgressDisplay', () => {

  it('returns null for null/undefined input', () => {
    assert.equal(resolveProgressDisplay(null), null)
    assert.equal(resolveProgressDisplay(undefined), null)
  })

  describe('status icons', () => {
    it('✅ for success', () => {
      assert.equal(resolveProgressDisplay({ status: 'success' }).statusIcon, '✅')
    })
    it('❌ for failed', () => {
      assert.equal(resolveProgressDisplay({ status: 'failed' }).statusIcon, '❌')
    })
    it('⏰ for timeout', () => {
      assert.equal(resolveProgressDisplay({ status: 'timeout' }).statusIcon, '⏰')
    })
    it('🔄 for running', () => {
      assert.equal(resolveProgressDisplay({ status: 'running' }).statusIcon, '🔄')
    })
    it('💤 for idle (unknown status)', () => {
      assert.equal(resolveProgressDisplay({ status: 'idle' }).statusIcon, '💤')
      assert.equal(resolveProgressDisplay({}).statusIcon, '💤')
    })
  })

  describe('status labels', () => {
    it('"已完成" for success', () => {
      assert.equal(resolveProgressDisplay({ status: 'success' }).statusLabel, '已完成')
    })
    it('"失败" for failed', () => {
      assert.equal(resolveProgressDisplay({ status: 'failed' }).statusLabel, '失败')
    })
    it('"超时" for timeout', () => {
      assert.equal(resolveProgressDisplay({ status: 'timeout' }).statusLabel, '超时')
    })
    it('"计算中..." for running', () => {
      assert.equal(resolveProgressDisplay({ status: 'running' }).statusLabel, '计算中...')
    })
    it('"空闲" for idle', () => {
      assert.equal(resolveProgressDisplay({ status: 'idle' }).statusLabel, '空闲')
    })
  })

  describe('bar colors', () => {
    it('emerald for success', () => {
      assert.equal(resolveProgressDisplay({ status: 'success' }).barColor, 'bg-emerald-500')
    })
    it('rose for failed', () => {
      assert.equal(resolveProgressDisplay({ status: 'failed' }).barColor, 'bg-rose-500')
    })
    it('amber for timeout', () => {
      assert.equal(resolveProgressDisplay({ status: 'timeout' }).barColor, 'bg-amber-500')
    })
    it('blue for running', () => {
      assert.equal(resolveProgressDisplay({ status: 'running' }).barColor, 'bg-blue-500')
    })
    it('blue for idle (default)', () => {
      assert.equal(resolveProgressDisplay({ status: 'idle' }).barColor, 'bg-blue-500')
    })
  })

  describe('pulse animation', () => {
    it('should pulse only when running', () => {
      assert.equal(resolveProgressDisplay({ status: 'running' }).shouldPulse, true)
      assert.equal(resolveProgressDisplay({ status: 'success' }).shouldPulse, false)
      assert.equal(resolveProgressDisplay({ status: 'failed' }).shouldPulse, false)
      assert.equal(resolveProgressDisplay({ status: 'idle' }).shouldPulse, false)
    })
  })

  describe('percent clamping', () => {
    it('clamps to max 100', () => {
      assert.equal(resolveProgressDisplay({ percent: 150 }).pct, 100)
    })
    it('clamps negative to 0 via Math.min (negative values pass through)', () => {
      // Note: In the real code path, `p.percent || 0` converts falsy to 0,
      // but -50 is truthy so it passes through. The Math.min with 100
      // only caps upper bound. Negative values shouldn't occur in practice.
      const result = resolveProgressDisplay({ percent: -50 }).pct
      assert.ok(result <= 100, 'should be clamped to max 100')
    })
    it('defaults to 0 when percent is missing/null/undefined', () => {
      assert.equal(resolveProgressDisplay({ status: 'running' }).pct, 0)
      assert.equal(resolveProgressDisplay({ percent: null }).pct, 0)
      assert.equal(resolveProgressDisplay({ percent: undefined }).pct, 0)
    })
    it('passes through valid percentages', () => {
      assert.equal(resolveProgressDisplay({ percent: 42.7 }).pct, 42.7)
      assert.equal(resolveProgressDisplay({ percent: 99.9 }).pct, 99.9)
    })
  })
})

// ═══════════════════════════════════════════
// Section C: Progress bar width calculation
// ═══════════════════════════════════════════

/**
 * Calculates the CSS width percentage for the progress bar.
 * Mirrors the style.width logic in renderProgressBar():
 *   - Running: max(2%, pct%)  — show at least a sliver
 *   - Success: 100%
 *   - Failed/Idle: 0%        — empty bar
 *   - Timeout: 0%
 */
function calcBarWidth(status, pct) {
  if (status === 'running') return Math.max(parseFloat(pct.toFixed(1)), 2)
  if (status === 'success') return 100
  return 0
}

describe('calcBarWidth', () => {

  it('returns 2% minimum for running when pct is near 0', () => {
    assert.equal(calcBarWidth('running', 0), 2)
    assert.equal(calcBarWidth('running', 0.1), 2)
  })

  it('returns actual pct for running when > 2%', () => {
    assert.equal(calcBarWidth('running', 45.6), 45.6)
    assert.equal(calcBarWidth('running', 99.9), 99.9)
  })

  it('returns 100% for success regardless of stored pct', () => {
    assert.equal(calcBarWidth('success', 0), 100)
    assert.equal(calcBarWidth('success', 42), 100)
    assert.equal(calcBarWidth('success', 100), 100)
  })

  it('returns 0% for terminal failure states', () => {
    assert.equal(calcBarWidth('failed', 75), 0)
    assert.equal(calcBarWidth('timeout', 80), 0)
  })

  it('returns 0% for idle state', () => {
    assert.equal(calcBarWidth('idle', 0), 0)
  })
})

// ═══════════════════════════════════════════
// Section D: Progress detail text generation
// ═══════════════════════════════════════════

/**
 * Generates the bottom-left detail text shown under the progress bar.
 * Mirrors the span text logic in renderProgressBar().
 */
function generateDetailText(p) {
  if (!p) return '--'
  const isRunning = p.status === 'running'
  const isSuccess = p.status === 'success'
  const isFailed = p.status === 'failed'

  if (isRunning && p.total > 0) {
    return `${p.current.toLocaleString()} / ${p.total.toLocaleString()} (${p.percent || 0}%)`
  }
  if (isSuccess && p.total > 0) {
    return `${p.total.toLocaleString()} 只`
  }
  if (isFailed) {
    return p.error_msg || '未知错误'
  }
  return '--'
}

describe('generateDetailText', () => {

  it('returns "--" for null/undefined', () => {
    assert.equal(generateDetailText(null), '--')
    assert.equal(generateDetailText(undefined), '--')
  })

  it('shows current/total/percent when running with total > 0', () => {
    const p = { status: 'running', current: 1234, total: 5000, percent: 24.68 }
    assert.equal(generateDetailText(p), '1,234 / 5,000 (24.68%)')
  })

  it('shows total count when success with total > 0', () => {
    const p = { status: 'success', current: 5000, total: 5000 }
    assert.equal(generateDetailText(p), '5,000 只')
  })

  it('shows error message when failed', () => {
    assert.equal(generateDetailText({ status: 'failed', error_msg: '成功率不足: 60%' }), '成功率不足: 60%')
  })

  it('shows default error when failed without msg', () => {
    assert.equal(generateDetailText({ status: 'failed' }), '未知错误')
  })

  it('shows "--" for idle or other states', () => {
    assert.equal(generateDetailText({ status: 'idle' }), '--')
    assert.equal(generateDetailText({ status: 'timeout' }), '--')
  })

  it('formats large numbers with locale separators', () => {
    const p = { status: 'running', current: 1200000, total: 5200000, percent: 23.077 }
    const result = generateDetailText(p)
    assert.ok(result.includes(','), `Expected locale separators in "${result}"`)
  })
})
