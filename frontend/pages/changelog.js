import { useMemo, useState } from 'react'

import changelogData from '../data/changelog.json'

const FILTER_OPTIONS = ['全部', '新功能', '修复优化', '工程维护']

const TYPE_STYLES = {
  新功能: 'border-sky-400/35 bg-sky-500/10 text-sky-200',
  修复优化: 'border-emerald-400/35 bg-emerald-500/10 text-emerald-200',
  工程维护: 'border-amber-400/35 bg-amber-500/10 text-amber-200',
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
      tone: 'text-white',
    },
    {
      label: '公开更新条数',
      value: `${items.length} 条`,
      tone: 'text-primary',
    },
  ]
}

export default function ChangelogPage() {
  const [activeFilter, setActiveFilter] = useState('全部')

  const allItems = useMemo(() => {
    const items = Array.isArray(changelogData?.items) ? changelogData.items : []

    return [...items]
      .filter((item) => item?.visible !== false)
      .sort((left, right) => String(right.date || '').localeCompare(String(left.date || '')))
  }, [])

  const filteredItems = useMemo(() => {
    if (activeFilter === '全部') return allItems
    return allItems.filter((item) => item.type === activeFilter)
  }, [activeFilter, allItems])

  const metaCards = useMemo(() => buildMetaCards(allItems), [allItems])

  return (
    <div className="space-y-6">
      <section className="rounded-2xl border border-border bg-card p-8">
        <div className="flex flex-col gap-6 lg:flex-row lg:items-end lg:justify-between">
          <div className="max-w-3xl">
            <div className="inline-flex rounded-full border border-primary/25 bg-primary/10 px-3 py-1 text-xs font-medium text-primary">
              产品迭代记录
            </div>
            <h1 className="mt-4 text-3xl font-semibold tracking-tight text-white">更新日志</h1>
            <p className="mt-3 text-sm leading-7 text-white/65">
              这里记录南瓜交易系统近期面向用户可感知的功能变化。
            </p>
          </div>

          <div className="grid gap-3 sm:grid-cols-3 lg:min-w-[420px]">
            {metaCards.map((card) => (
              <div key={card.label} className="rounded-2xl border border-white/10 bg-black/20 px-4 py-4">
                <div className="text-xs text-white/45">{card.label}</div>
                <div className={`mt-2 text-lg font-semibold ${card.tone}`}>{card.value}</div>
              </div>
            ))}
          </div>
        </div>

        {/* <div className="mt-6 rounded-2xl border border-dashed border-white/10 bg-black/15 px-4 py-4 text-sm text-white/60">
          后续维护方式已经收敛成手动追加：只需要按统一字段往更新日志数据里补一条中文记录，不再依赖自动同步脚本。这样可控，也不容易把原始提交记录流水账误当成产品更新。
        </div> */}
      </section>

      <section className="rounded-2xl border border-border bg-card p-5">
        <div className="flex flex-col gap-4 lg:flex-row lg:items-center lg:justify-between">
          <div>
            <h2 className="text-base font-semibold text-white">最近更新</h2>
            <p className="mt-1 text-xs text-white/55">默认只展示公开可见条目，可按类型快速筛选。</p>
          </div>

          <div className="flex flex-wrap gap-2">
            {FILTER_OPTIONS.map((option) => {
              const isActive = activeFilter === option
              return (
                <button
                  key={option}
                  type="button"
                  onClick={() => setActiveFilter(option)}
                  className={`rounded-full border px-3 py-1.5 text-xs transition ${
                    isActive
                      ? 'border-primary bg-primary/10 text-primary'
                      : 'border-white/10 text-white/65 hover:border-white/20 hover:text-white'
                  }`}
                >
                  {option}
                </button>
              )
            })}
          </div>
        </div>

        {filteredItems.length ? (
          <div className="mt-5 space-y-4">
            {filteredItems.map((item) => (
              <article key={`${item.date}-${item.title}`} className="rounded-2xl border border-white/10 bg-black/20 p-5">
                <div className="flex flex-col gap-3 lg:flex-row lg:items-start lg:justify-between">
                  <div className="space-y-3">
                    <div className="flex flex-wrap items-center gap-2">
                      <span className={`inline-flex rounded-full border px-2.5 py-1 text-xs font-medium ${TYPE_STYLES[item.type] || 'border-white/15 bg-white/5 text-white/75'}`}>
                        {item.type}
                      </span>
                      {item.scope ? (
                        <span className="inline-flex rounded-full border border-white/10 bg-white/5 px-2.5 py-1 text-xs text-white/55">
                          {item.scope}
                        </span>
                      ) : null}
                    </div>

                    <div>
                      <h3 className="text-lg font-semibold text-white">{item.title}</h3>
                      <p className="mt-2 text-sm leading-7 text-white/68">{item.summary}</p>
                    </div>
                  </div>

                  <div className="shrink-0 rounded-2xl border border-white/10 bg-black/25 px-4 py-3 text-right text-xs text-white/50">
                    <div>{formatDisplayDate(item.date)}</div>
                  </div>
                </div>
              </article>
            ))}
          </div>
        ) : (
          <div className="mt-5 rounded-2xl border border-dashed border-white/10 px-4 py-8 text-center text-sm text-white/45">
            当前筛选条件下还没有可展示的更新，先别急，产品没有偷偷摸鱼。
          </div>
        )}
      </section>
    </div>
  )
}
