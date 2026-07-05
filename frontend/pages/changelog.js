import { useMemo } from 'react'
import Head from 'next/head'
import changelogData from '../data/changelog.json'
import CommunityQRCard from '../components/CommunityQRCard'

const TYPE_STYLES = {
  新功能: 'border-sky-500/30 dark:border-sky-400/50 bg-sky-100 dark:bg-sky-500/30 text-sky-700 dark:text-sky-100 shadow-none dark:shadow-[0_10px_30px_rgba(56,189,248,0.12)]',
  修复优化: 'border-emerald-500/30 dark:border-emerald-400/50 bg-emerald-100 dark:bg-emerald-500/30 text-emerald-700 dark:text-emerald-100 shadow-none dark:shadow-[0_10px_30px_rgba(16,185,129,0.12)]',
  工程维护: 'border-amber-500/30 dark:border-amber-400/35 bg-amber-100 dark:bg-amber-500/12 text-amber-700 dark:text-amber-100 shadow-none dark:shadow-[0_10px_30px_rgba(245,158,11,0.12)]',
}

function formatDisplayDate(value) {
  if (!value) return '--'

  const [year, month, day] = String(value).split('-')
  if (!year || !month || !day) return value
  return `${year}年${Number(month)}月${Number(day)}日`
}

function buildMetaCards(items) {
  return [
    {
      label: '最近更新时间',
      value: formatDisplayDate(items[0]?.date || changelogData.last_updated),
      tone: 'text-foreground',
    },
    {
      label: '更新日志条数',
      value: `${items.length} 条`,
      tone: 'text-primary',
    },
  ]
}

function buildDateGroups(items) {
  const groups = new Map()

  items.forEach((item) => {
    const date = item?.date || '未标注日期'
    if (!groups.has(date)) {
      groups.set(date, [])
    }
    groups.get(date).push(item)
  })

  return Array.from(groups.entries()).map(([date, groupItems]) => ({
    date,
    items: groupItems,
  }))
}

export default function ChangelogPage() {
  const allItems = useMemo(() => {
    const items = Array.isArray(changelogData?.items) ? changelogData.items : []

    return [...items]
      .filter((item) => item?.visible !== false)
      .sort((left, right) => String(right.date || '').localeCompare(String(left.date || '')))
  }, [])

  const metaCards = useMemo(() => buildMetaCards(allItems), [allItems])
  const groupedItems = useMemo(() => buildDateGroups(allItems), [allItems])

  return (
    <div className="space-y-6 pb-8">
      <Head>
        <title>更新日志 — 卧龙AI量化交易台</title>
        <meta name="description" content="卧龙AI量化交易台更新日志 — 查看最新功能、优化与修复记录。" />
        <link rel="canonical" href="https://wolongtrader.top/changelog" />
      </Head>
      <section className="relative overflow-hidden rounded-[28px] border border-border bg-[var(--color-bg-secondary)] dark:bg-[#111114] p-8 shadow-[0_24px_80px_rgba(0,0,0,0.08)] dark:shadow-[0_24px_80px_rgba(0,0,0,0.35)] lg:p-10">
        <div className="absolute inset-0 bg-[radial-gradient(circle_at_top_left,rgba(230,126,34,0.18),transparent_34%),radial-gradient(circle_at_85%_20%,rgba(56,189,248,0.14),transparent_24%),linear-gradient(135deg,rgba(255,255,255,0.04),transparent_55%)]" />
        <div className="absolute inset-y-0 right-0 hidden w-1/3 bg-[linear-gradient(180deg,rgba(255,255,255,0.06),transparent_70%)] lg:block" />

        <div className="relative flex flex-col gap-8 xl:flex-row xl:items-end xl:justify-between">
          <div className="max-w-3xl">
            <div className="inline-flex rounded-full border border-primary/25 bg-primary/10 px-3 py-1 text-xs tracking-[0.22em] text-primary">
              产品迭代记录
            </div>
            <h1
              className="mt-5 max-w-2xl text-4xl leading-tight text-foreground md:text-5xl"
              style={{ fontFamily: 'Iowan Old Style, Palatino Linotype, Times New Roman, serif' }}
            >
              产品更新日志
            </h1>
            <p className="mt-5 max-w-2xl text-sm leading-7 text-foreground/68 md:text-base">
              如果你有什么想要的功能可以在设置页面提交反馈告诉我们
            </p>
            <CommunityQRCard
              variant="inline"
              className="mt-6 max-w-md"
            />
          </div>

          <div className="grid gap-3 sm:grid-cols-2 xl:min-w-[420px]">
            {metaCards.map((card) => (
              <div key={card.label} className="rounded-[24px] border border-border bg-[var(--color-bg-secondary)] px-5 py-5 backdrop-blur-sm">
                <div className="text-[11px] uppercase tracking-[0.22em] text-foreground-dim">{card.label}</div>
                <div className={`mt-3 text-2xl font-semibold ${card.tone}`}>{card.value}</div>
                {card.hint ? <div className="mt-2 text-xs leading-6 text-foreground/42">{card.hint}</div> : null}
              </div>
            ))}
          </div>
        </div>
      </section>

      <section className="rounded-[28px] border border-border bg-card/95 p-5 shadow-[0_18px_50px_rgba(0,0,0,0.22)]">
        <div className="border-b border-border pb-5">
          <h2 className="text-lg font-semibold text-foreground">最近更新</h2>
        </div>

        {groupedItems.length ? (
          <div className="mt-6 space-y-8">
            {groupedItems.map((group) => (
              <section key={group.date} className="grid gap-4 lg:grid-cols-[180px_minmax(0,1fr)] lg:gap-6">
                <div className="lg:sticky lg:top-24 lg:self-start">
                  <div className="rounded-[22px] border border-border bg-[var(--color-bg-hover)] px-4 py-4">
                    <div className="text-[11px] uppercase tracking-[0.22em] text-foreground/36">更新日期</div>
                    <div
                      className="mt-2 text-2xl text-foreground"
                      style={{ fontFamily: 'Iowan Old Style, Palatino Linotype, Times New Roman, serif' }}
                    >
                      {formatDisplayDate(group.date)}
                    </div>
                  </div>
                </div>

                <div className="relative space-y-4 pl-0 lg:pl-8">
                  <div className="absolute left-0 top-0 hidden h-full w-px bg-gradient-to-b from-primary/35 via-[var(--color-border)] to-transparent lg:block" />

                  {group.items.map((item, index) => (
                    <article
                      key={`${item.date}-${item.title}`}
                      className="relative overflow-hidden rounded-[24px] border border-border bg-[linear-gradient(180deg,var(--color-bg-hover),transparent)] p-5 shadow-[0_18px_40px_rgba(0,0,0,0.2)] transition hover:-translate-y-0.5 hover:border-[var(--color-border-strong)] hover:shadow-[0_24px_56px_rgba(0,0,0,0.28)]"
                    >
                      <div className="absolute left-0 top-8 hidden h-3 w-3 -translate-x-[37px] rounded-full border border-primary/35 bg-background shadow-[0_0_0_6px_rgba(230,126,34,0.08)] lg:block" />

                      <div className="flex flex-col gap-4 lg:flex-row lg:items-start lg:justify-between">
                        <div className="space-y-3">
                          <div className="flex flex-wrap items-center gap-2">
                            <span className={`inline-flex rounded-full border px-2.5 py-1 text-xs font-medium ${TYPE_STYLES[item.type] || 'border-[var(--color-border-strong)] bg-[var(--color-bg-hover)] text-foreground-muted'}`}>
                              {item.type}
                            </span>
                            {item.scope ? (
                              <span className="inline-flex rounded-full border border-border bg-[var(--color-bg-hover)] px-2.5 py-1 text-xs text-foreground-dim">
                                {item.scope}
                              </span>
                            ) : null}
                            <span className="text-[11px] uppercase tracking-[0.24em] text-foreground/28">第 {String(index + 1).padStart(2, '0')} 条</span>
                          </div>

                          <div>
                            <h3
                              className="text-[24px] leading-tight text-foreground"
                              style={{ fontFamily: 'Iowan Old Style, Palatino Linotype, Times New Roman, serif' }}
                            >
                              {item.title}
                            </h3>
                            <p className="mt-3 max-w-3xl text-sm leading-7 text-foreground/66 md:text-[15px]">
                              {item.summary}
                            </p>
                          </div>
                        </div>

                        <div className="shrink-0 rounded-2xl border border-border bg-[var(--color-bg-hover)] px-4 py-3 text-left text-xs text-foreground/48 lg:min-w-[120px] lg:text-right">
                          <div className="uppercase tracking-[0.22em] text-foreground/28">记录时间</div>
                          <div className="mt-2 text-sm text-foreground/62">{formatDisplayDate(item.date)}</div>
                        </div>
                      </div>
                    </article>
                  ))}
                </div>
              </section>
            ))}
          </div>
        ) : (
          <div className="mt-6 rounded-[24px] border border-dashed border-border px-4 py-10 text-center text-sm text-foreground-dim">
            暂时还没有可展示的更新，先别急，产品没有偷偷摸鱼。
          </div>
        )}
      </section>
    </div>
  )
}
