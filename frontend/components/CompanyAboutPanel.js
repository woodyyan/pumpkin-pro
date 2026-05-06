import { extractDisplayDomain, formatAboutDate, formatListingStatus, listingStatusTone, normalizeWebsiteHref } from '../lib/company-about'

function AboutField({ label, value, children }) {
  const content = children || value || '--'
  return (
    <div className="rounded-xl border border-white/8 bg-black/15 px-3 py-3">
      <div className="text-[11px] text-white/38">{label}</div>
      <div className="mt-1 break-words text-sm font-medium text-white/82">{content}</div>
    </div>
  )
}

function statusClass(status) {
  const tone = listingStatusTone(status)
  if (tone === 'listed') return 'border-emerald-300/25 bg-emerald-500/10 text-emerald-200'
  if (tone === 'delisted') return 'border-amber-300/30 bg-amber-500/10 text-amber-200'
  if (tone === 'suspended') return 'border-sky-300/30 bg-sky-500/10 text-sky-200'
  return 'border-white/10 bg-white/[0.04] text-white/55'
}

function formatUpdatedAt(value) {
  if (!value) return ''
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return date.toLocaleString('zh-CN', { hour12: false })
}

export default function CompanyAboutPanel({ payload, loading = false, error = '', onClose }) {
  const profile = payload?.profile || null
  const meta = payload?.meta || {}
  const status = profile?.listing_status || 'UNKNOWN'
  const websiteHref = normalizeWebsiteHref(profile?.website)
  const websiteDomain = extractDisplayDomain(profile?.website)
  const updatedText = formatUpdatedAt(meta.updated_at || meta.source_updated_at)

  return (
    <section className="rounded-2xl border border-border bg-card p-5" data-company-about-panel="true">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <div className="flex flex-wrap items-center gap-2">
            <h3 className="text-base font-semibold text-white">关于这家公司</h3>
            {!loading && !error && profile ? (
              <span className={`rounded-full border px-2.5 py-1 text-[11px] font-medium ${statusClass(status)}`}>
                {formatListingStatus(status)}
              </span>
            ) : null}
          </div>
          <p className="mt-1 text-xs text-white/45">
            {meta.source ? `来源：${meta.source}` : '静态公司资料'}{updatedText ? ` · 更新：${updatedText}` : ''}
          </p>
        </div>
        {onClose ? (
          <button
            type="button"
            onClick={onClose}
            className="rounded-lg border border-white/10 px-2.5 py-1 text-xs text-white/50 transition hover:border-white/20 hover:text-white/75"
          >
            收起
          </button>
        ) : null}
      </div>

      {loading ? (
        <div className="mt-4 rounded-xl border border-dashed border-border px-4 py-6 text-sm text-white/50">公司资料加载中...</div>
      ) : error ? (
        <div className="mt-4 rounded-xl border border-amber-400/30 bg-amber-500/10 px-4 py-3 text-sm text-amber-200">{error}</div>
      ) : !payload?.has_profile ? (
        <div className="mt-4 rounded-xl border border-dashed border-border px-4 py-6 text-sm text-white/55">
          {meta.message || '资料整理中，暂未收录该公司的静态资料。'}
        </div>
      ) : (
        <div className="mt-4 grid gap-4 lg:grid-cols-[minmax(0,1.15fr)_minmax(320px,0.85fr)]">
          <div className="rounded-xl border border-white/8 bg-black/15 p-4">
            <div className="text-[11px] text-white/38">一句话业务介绍</div>
            <p className="mt-2 text-sm leading-7 text-white/82">
              {profile?.business_summary || '该公司的业务介绍暂待补全。'}
            </p>
            {status === 'DELISTED' ? (
              <div className="mt-3 rounded-lg border border-amber-300/25 bg-amber-500/10 px-3 py-2 text-xs leading-5 text-amber-100/85">
                该证券可能已退市，资料保留为最后一次收录信息。
              </div>
            ) : null}
            {profile?.business_scope ? (
              <details className="mt-3 group">
                <summary className="cursor-pointer text-xs text-white/45 transition hover:text-white/70">查看原始业务资料</summary>
                <p className="mt-2 text-xs leading-6 text-white/55">{profile.business_scope}</p>
              </details>
            ) : null}
          </div>

          <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-1 xl:grid-cols-2">
            <AboutField label="板块" value={profile?.board_name} />
            <AboutField label="行业" value={profile?.industry_name} />
            <AboutField label="官网">
              {websiteHref ? (
                <a className="text-primary transition hover:text-primary/80" href={websiteHref} target="_blank" rel="noreferrer">
                  {websiteDomain || profile.website}
                </a>
              ) : '--'}
            </AboutField>
            <AboutField label="成立时间" value={formatAboutDate(profile?.founded_date, profile?.founded_date_precision)} />
            <AboutField label="IPO 日期" value={formatAboutDate(profile?.ipo_date)} />
            <AboutField label="行业来源" value={profile?.industry_source || profile?.raw_industry_name} />
          </div>
        </div>
      )}
    </section>
  )
}
