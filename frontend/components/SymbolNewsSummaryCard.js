import { buildNewsHeadlineText, buildNewsSummaryBadges, formatNewsTime } from '../lib/symbol-news-ui'

export default function SymbolNewsSummaryCard({
  summary,
  updatedAt,
  loading = false,
  error = '',
  onOpen,
}) {
  const badges = buildNewsSummaryBadges(summary)
  const headline = buildNewsHeadlineText(summary)

  return (
    <section className="rounded-2xl border border-border bg-card p-5">
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0 flex-1">
          <div className="flex flex-wrap items-center gap-2">
            <h3 className="text-base font-semibold text-white">新闻与公告</h3>
            {updatedAt ? <span className="text-[11px] text-white/40">更新：{formatNewsTime(updatedAt)}</span> : null}
          </div>
          <div className="mt-2 flex flex-wrap gap-2">
            {badges.map((badge) => (
              <span key={badge} className="rounded-full border border-white/10 bg-white/[0.04] px-2.5 py-1 text-[11px] text-white/65">
                {badge}
              </span>
            ))}
          </div>
          <div className="mt-3 text-sm leading-6 text-white/75">{headline}</div>
          {error ? <div className="mt-3 rounded-lg border border-amber-400/25 bg-amber-500/10 px-3 py-2 text-xs text-amber-200">{error}</div> : null}
        </div>
        <button
          type="button"
          onClick={onOpen}
          className="shrink-0 inline-flex items-center gap-1 rounded-lg border border-primary/40 bg-primary/10 px-2.5 py-1 text-xs font-medium text-primary transition hover:bg-primary/20"
        >
          {loading ? '加载中...' : '查看全部 →'}
        </button>
      </div>
    </section>
  )
}
