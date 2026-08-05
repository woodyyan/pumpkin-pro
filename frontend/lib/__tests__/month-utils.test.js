import test from 'node:test'
import assert from 'node:assert/strict'
import { getCurrentMonth, shiftMonth } from '../month-utils.js'

const MONTH_RE = /^\d{4}-\d{2}$/

// 时区翻转辅助：Node 在 macOS/Linux 上运行期改 TZ 生效，用于证明 shiftMonth 与时区无关。
function withTZ(tz, fn) {
  const prev = process.env.TZ
  process.env.TZ = tz
  try {
    return fn()
  } finally {
    process.env.TZ = prev
  }
}

test('getCurrentMonth 返回本地 YYYY-MM', () => {
  assert.equal(getCurrentMonth(new Date(2026, 7, 5)), '2026-08')
  assert.equal(getCurrentMonth(new Date(2026, 0, 1)), '2026-01')
  assert.equal(getCurrentMonth(new Date(2026, 11, 31)), '2026-12')
  assert.equal(getCurrentMonth(new Date(2026, 2, 15)), '2026-03') // 月份补零
})

test('shiftMonth 正常偏移', () => {
  assert.equal(shiftMonth('2026-08', -1), '2026-07')
  assert.equal(shiftMonth('2026-08', 1), '2026-09')
  assert.equal(shiftMonth('2026-08', 0), '2026-08')
})

test('shiftMonth 跨年偏移', () => {
  assert.equal(shiftMonth('2026-01', -1), '2025-12')
  assert.equal(shiftMonth('2026-12', 1), '2027-01')
  assert.equal(shiftMonth('2025-12', 12), '2026-12')
})

test('shiftMonth 输出始终符合 YYYY-MM 格式', () => {
  for (const m of ['2026-01', '2026-06', '2026-12']) {
    for (const d of [-1, 1, -2, 13]) {
      assert.match(shiftMonth(m, d), MONTH_RE)
    }
  }
})

test('shiftMonth 非法输入回退当前月，不抛错', () => {
  assert.match(shiftMonth(undefined, 1), MONTH_RE)
  assert.match(shiftMonth(null, 1), MONTH_RE)
  assert.match(shiftMonth('garbage', 1), MONTH_RE)
  assert.match(shiftMonth('2026-13', -1), MONTH_RE) // 月份越界
  assert.match(shiftMonth('2026-08', 'not-a-number'), MONTH_RE)
})

test('shiftMonth 结果不随本地时区漂移（东八区回归用例）', () => {
  // 旧实现：8月-1 → 6月、8月+1 → 8月（本地解析 + UTC 序列化混用）。
  // 新实现：任意时区下结果一致。
  const cases = [
    ['2026-08', -1, '2026-07'],
    ['2026-08', 1, '2026-09'],
    ['2026-08', -2, '2026-06'],
    ['2026-06', 1, '2026-07'],
  ]
  for (const tz of ['Asia/Shanghai', 'UTC', 'America/New_York']) {
    withTZ(tz, () => {
      for (const [month, delta, expected] of cases) {
        assert.equal(shiftMonth(month, delta), expected, `TZ=${tz} month=${month} delta=${delta}`)
      }
    })
  }
})

test('getCurrentMonth 随本地时区取当月（东八区 1 号凌晨不落后一个月）', () => {
  // 旧实现用 new Date().toISOString()：东八区每月 1 号 00:00-08:00 会返回上个月。
  // 新实现按本地 getFullYear/getMonth 取当月。
  withTZ('Asia/Shanghai', () => {
    assert.equal(getCurrentMonth(new Date(2026, 7, 1, 0, 30)), '2026-08')
    assert.equal(getCurrentMonth(new Date(2026, 7, 1, 7, 59)), '2026-08')
  })
  withTZ('America/New_York', () => {
    assert.equal(getCurrentMonth(new Date(2026, 7, 1, 0, 30)), '2026-08')
  })
})
