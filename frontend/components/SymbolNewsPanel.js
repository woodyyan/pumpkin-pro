import { buildNewsEmptyState, filterSymbolNewsItems, formatNewsTime, formatNewsTypeLabel } from '../lib/symbol-news-ui'

const FILTERS = [
  { value: 'all', label: '全部' },
  { value: 'news', label: '新闻' },
  { value: 'announcement', label: '公告' },
  { value: 'filing', label: '财报' },
]

export default function SymbolNewsPanel({
  open,
  items,
  summary,
  activeType,
  loading = false,
  error = '',
  updatedAt = '',
  onClose,
  onRefresh,
  onTypeChange,
}) {
  if (!open) return null

  const filtered = filterSymbolNewsItems(items, activeType)

  return (
    <div className="fixed inset-0 z-[70]">
      <div className="absolute inset-0 bg-[var(--color-bg-overlay)] backdrop-blur-[2px]" onClick={onClose} />
      <div className="absolute inset-x-0 bottom-0 top-16 overflow-hidden rounded-t-[28px] border border-border bg-white dark:bg-[#0f1117] shadow-2xl md:inset-y-0 md:right-0 md:left-auto md:top-0 md:w-[460px] md:rounded-none md:border-l md:border-t-0 md:border-r-0 md:border-b-0">
        <div className="flex h-full flex-col">
          <div className="border-b border-border px-4 py-4 sm:px-5">
            <div className="flex items-start justify-between gap-3">
              <div className="min-w-0 flex-1">
                <h3 className="text-base font-semibold text-foreground">新闻与公告</h3>
                <div className="mt-1 text-xs text-foreground-dim">
                  {summary?.last_24h_count ? `近24h ${summary.last_24h_count} 条` : '暂无高相关事件'}
                  {updatedAt ? ` · 更新：${formatNewsTime(updatedAt)}` : ''}
                </div>
              </div>
              <div className="flex items-center gap-2">
                <button
                  type="button"
                  onClick={onRefresh}
                  className="rounded-lg border border-border px-2.5 py-1.5 text-xs text-foreground-muted transition hover:border-[var(--color-border-strong)] hover:text-foreground"
                >
                  刷新
                </button>
                <button
                  type="button"
                  onClick={onClose}
                  className="rounded-lg border border-border px-2.5 py-1.5 text-xs text-foreground-muted transition hover:border-[var(--color-border-strong)] hover:text-foreground"
                >
                  关闭
                </button>
              </div>
            </div>
            {Array.isArray(summary?.highlight_tags) && summary.highlight_tags.length > 0 ? (
              <div className="mt-3 flex gap-2 overflow-x-auto pb-1">
                {summary.highlight_tags.map((tag) => (
                  <span key={tag} className="whitespace-nowrap rounded-full border border-primary/20 bg-primary/10 px-2.5 py-1 text-[11px] text-primary">
                    {tag}
                  </span>
                ))}
              </div>
            ) : null}
            <div className="mt-3 flex gap-2 overflow-x-auto pb-1">
              {FILTERS.map((item) => (
                <button
                  key={item.value}
                  type="button"
                  onClick={() => onTypeChange(item.value)}
                  className={`whitespace-nowrap rounded-full border px-3 py-1.5 text-xs transition ${activeType === item.value ? 'border-primary bg-primary text-foreground' : 'border-border bg-[var(--color-bg-hover)] text-foreground-muted hover:border-[var(--color-border-strong)] hover:text-foreground'}`}
                >
                  {item.label}
                </button>
              ))}
            </div>
          </div>

          <div className="flex-1 overflow-y-auto px-4 py-4 sm:px-5">
            {error ? <div className="rounded-lg border border-amber-400/25 dark:border-amber-400/25 bg-amber-50 dark:bg-amber-500/10 px-3 py-2 text-xs text-amber-800 dark:text-amber-200">{error}</div> : null}
            {loading ? (
              <div className="rounded-xl border border-dashed border-border px-4 py-8 text-sm text-foreground-dim">新闻加载中...</div>
            ) : filtered.length === 0 ? (
              <div className="rounded-xl border border-dashed border-border px-4 py-8 text-sm text-foreground-dim">{buildNewsEmptyState(activeType)}</div>
            ) : (
              <div className="space-y-3">
                {filtered.map((item) => (
                  <article key={item.id} className="rounded-2xl border border-border bg-[var(--color-bg-hover)] p-4">
                    <div className="flex flex-wrap items-center gap-2 text-[11px] text-foreground-dim">
                      <span className={`rounded-full border px-2 py-0.5 ${item.type === 'filing' ? 'border-primary/30 bg-primary/10 text-primary' : item.type === 'announcement' ? 'border-amber-300/30 bg-amber-500/10 text-amber-200' : 'border-border bg-[var(--color-bg-hover)] text-foreground-dim'}`}>
                        {formatNewsTypeLabel(item.type)}
                      </span>
                      {(item.source_type === 'official' || item.official) ? <span className="rounded-full border border-emerald-300/25 bg-positive/10 px-2 py-0.5 text-positive">官方</span> : null}
                      <span>{item.source_name || item.source || '未知来源'}</span>
                      <span>{formatNewsTime(item.published_at)}</span>
                    </div>
                    <div className="mt-3 text-sm font-medium leading-6 text-foreground">{item.title}</div>
                    {item.summary ? <div className="mt-2 text-sm leading-6 text-foreground-muted">{item.summary}</div> : null}
                    {(item.report_period || item.report_type || item.url) ? (
                      <div className="mt-3 flex flex-wrap items-center gap-3 text-xs text-foreground-dim">
                        {item.report_period ? <span>{item.report_period}</span> : null}
                        {item.report_type ? <span>{item.report_type}</span> : null}
                        {item.url ? <a href={item.url} target="_blank" rel="noreferrer" className="text-primary hover:text-primary/80">原文</a> : null}
                      </div>
                    ) : null}
                  </article>
                ))}
              </div>
            )}
          </div>
        </div>
      </div>
    </div>
  )
}
