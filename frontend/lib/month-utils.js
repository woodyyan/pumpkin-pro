// 月份工具：市场日历驾驶舱等场景的 YYYY-MM 字符串运算。
//
// 时区约定（重要）：
// - shiftMonth 全程使用 UTC（解析带 Z、setUTCMonth、toISOString 序列化），
//   避免“本地时区解析 + UTC 序列化”导致的月份错位（东八区下输出恒比预期少 1 月）。
// - getCurrentMonth 按本地时区取当前月，供“本月”按钮与初始 state 使用。
const MONTH_PATTERN = /^\d{4}-(0[1-9]|1[0-2])$/

function padMonth(month) {
  return String(month).padStart(2, '0')
}

/**
 * 取本地当前年月，格式 YYYY-MM。
 * @param {Date} [date] 可注入日期便于测试，默认当前时间。
 */
export function getCurrentMonth(date = new Date()) {
  return `${date.getFullYear()}-${padMonth(date.getMonth() + 1)}`
}

/**
 * 月份偏移：YYYY-MM + delta → YYYY-MM，支持跨年（2026-01 + -1 → 2025-12）。
 * 非法 month / delta 输入回退到当前月，绝不抛错。
 * @param {string} [month] YYYY-MM，缺省或非法时按当前月计算。
 * @param {number} [delta] 月偏移量，默认 0。
 */
export function shiftMonth(month, delta) {
  const base = typeof month === 'string' && MONTH_PATTERN.test(month)
    ? new Date(`${month}-01T00:00:00Z`)
    : new Date()
  const step = Number(delta)
  base.setUTCMonth(base.getUTCMonth() + (Number.isFinite(step) ? step : 0))
  return base.toISOString().slice(0, 7)
}
