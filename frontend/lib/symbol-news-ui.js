export function buildNewsSummaryBadges(summary) {
  if (!summary || typeof summary !== 'object') return []
  const badges = []
  const last24h = Number(summary.last_24h_count || 0)
  const announcements = Number(summary.announcement_count || 0)
  const filings = Number(summary.filing_count || 0)

  if (last24h > 0) badges.push(`近24h ${last24h}条`)
  if (announcements > 0) badges.push(`公告 ${announcements}条`)
  if (filings > 0) badges.push(`财报 ${filings}份`)
  if (badges.length === 0) badges.push('暂无相关新闻')
  return badges
}

export function buildNewsHeadlineText(summary) {
  const headline = String(summary?.latest_headline || '').trim()
  if (headline) return headline
  return '最近暂无高相关的新闻、公告或财报更新。'
}

export function filterSymbolNewsItems(items, activeType = 'all') {
  const list = Array.isArray(items) ? items : []
  const wanted = String(activeType || 'all').trim().toLowerCase()
  if (!wanted || wanted === 'all') return list
  return list.filter((item) => String(item?.type || '').trim().toLowerCase() === wanted)
}

export function formatNewsTypeLabel(type) {
  const key = String(type || '').trim().toLowerCase()
  if (key === 'announcement') return '公告'
  if (key === 'filing') return '财报'
  return '新闻'
}

export function formatNewsTime(value) {
  if (!value) return '--'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return String(value)
  return date.toLocaleString('zh-CN', {
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    hour12: false,
  })
}

export function buildNewsEmptyState(activeType = 'all') {
  const wanted = String(activeType || 'all').trim().toLowerCase()
  if (wanted === 'announcement') return '当前没有可展示的公告。'
  if (wanted === 'filing') return '当前没有可展示的财报。'
  if (wanted === 'news') return '当前没有可展示的媒体新闻。'
  return '当前没有可展示的新闻、公告或财报。'
}

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
