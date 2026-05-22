/**
 * trade-date-label — unified helper for rendering source_trade_date
 *
 * All "daily close" products (quadrant, ranking, portfolio) should use this
 * helper to display their data-date label so the user always sees the real
 * closing date rather than the batch computation time.
 *
 * Backend provides `source_trade_date` (e.g. "2026-05-21") in the metadata
 * of each batch. If the field is missing (e.g. old API version), we fall
 * back to `computed_at` but still format it as a date-only string.
 */

/**
 * Format a source_trade_date value into the canonical user-facing label.
 *
 * @param {string|null|undefined} sourceTradeDate - e.g. "2026-05-21"
 * @param {string|null|undefined} fallbackComputedAt - e.g. "2026-05-22T02:30:00Z"
 * @returns {string} e.g. "按 2026/5/21 收盘后数据生成" or empty string
 */
export function formatCloseDateLabel(sourceTradeDate, fallbackComputedAt) {
  const dateStr = extractDateString(sourceTradeDate) || extractDateString(fallbackComputedAt)
  if (!dateStr) return ''
  return `按 ${formatCompactDate(dateStr)} 收盘后数据生成`
}

/**
 * Extract a YYYY-MM-DD date string from various input shapes.
 * Accepts: "2026-05-21", "2026-05-22T02:30:00Z", ISO Date objects, etc.
 *
 * @param {string|null|undefined} value
 * @returns {string|null} "2026-05-21" or null
 */
function extractDateString(value) {
  if (!value) return null

  // Already a clean date string
  if (typeof value === 'string') {
    const match = value.match(/^(\d{4}-\d{2}-\d{2})/)
    return match ? match[1] : null
  }

  return null
}

/**
 * Format "2026-05-21" → "2026/5/21" (no zero-padding on month/day).
 *
 * @param {string} dateStr - YYYY-MM-DD
 * @returns {string}
 */
export function formatCompactDate(dateStr) {
  if (!dateStr || !/^\d{4}-\d{2}-\d{2}$/.test(dateStr)) return dateStr || ''
  const [year, month, day] = dateStr.split('-')
  return `${year}/${Number(month)}/${Number(day)}`
}

export function parseTradeDateLabelDate(value) {
  return extractDateString(value) || ""
}
