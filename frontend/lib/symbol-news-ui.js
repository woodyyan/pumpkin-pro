// 新闻与公告 Tab 已下线（2026-08-15）；本文件仅保留 AI 分析上下文构建，
// 供个股页 AI 面板与 /ai/analysis 页通过 fetchAIAnalysisNewsContext 使用。
export function buildAINewsContext({ summary, items, maxItems = 6 } = {}) {
  const safeSummary = summary && typeof summary === 'object' ? summary : null
  const safeItems = Array.isArray(items) ? items : []
  const normalizedItems = safeItems
    .filter((item) => item && typeof item === 'object' && String(item.title || '').trim())
    .slice(0, maxItems)
    .map((item) => ({
      type: String(item.type || 'news').trim().toLowerCase() || 'news',
      source: String(item.source_name || item.source || '').trim(),
      published_at: String(item.published_at || '').trim(),
      title: String(item.title || '').trim(),
      summary: String(item.summary || '').trim(),
      official: String(item.source_type || '').trim().toLowerCase() === 'official' || Boolean(item.official),
      report_period: String(item.report_period || '').trim(),
      report_type: String(item.report_type || '').trim(),
    }))

  const normalizedSummary = safeSummary
    ? {
        last_24h_count: Number(safeSummary.last_24h_count || 0),
        announcement_count: Number(safeSummary.announcement_count || 0),
        filing_count: Number(safeSummary.filing_count || 0),
        latest_headline: String(safeSummary.latest_headline || '').trim(),
        highlight_tags: Array.isArray(safeSummary.highlight_tags)
          ? safeSummary.highlight_tags.map((item) => String(item || '').trim()).filter(Boolean).slice(0, 6)
          : [],
      }
    : null

  const valid = Boolean(
    normalizedItems.length > 0 ||
    (normalizedSummary && (
      normalizedSummary.last_24h_count > 0 ||
      normalizedSummary.announcement_count > 0 ||
      normalizedSummary.filing_count > 0 ||
      normalizedSummary.latest_headline ||
      normalizedSummary.highlight_tags.length > 0
    ))
  )

  if (!valid) return { _valid: false }

  return {
    _valid: true,
    summary: normalizedSummary || {
      last_24h_count: 0,
      announcement_count: 0,
      filing_count: 0,
      latest_headline: '',
      highlight_tags: [],
    },
    items: normalizedItems,
  }
}
