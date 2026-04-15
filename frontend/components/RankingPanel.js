import { useCallback } from 'react'

/**
 * RankingPanel — 卧龙AI精选排行榜
 * 从四象限机会区中筛选 Top N 股票，按 Opportunity 降序排列。
 *
 * @param {Object} props
 * @param {Array|null}  props.items     - 排行榜数据列表（RankingItem[]）
 * @param {Object|null} props.meta      - 元数据 { computed_at, total_in_zone, returned_count, exchange }
 * @param {boolean}     props.loading   - 加载态
 * @param {string}      props.exchange  - 当前市场 'ASHARE' | 'HKEX'
 * @param {string}      props.onExchangeChange - Tab 切换回调
 */
export default function RankingPanel({ items = [], meta = null, loading = false, exchange = 'ASHARE', onExchangeChange }) {
  const handleItemClick = useCallback((code, exchange) => {
    const sym =
      exchange === 'HKEX'
        ? code.padStart(5, '0') + '.HK'
        : (code.length === 6
            ? (code[0] === '6' ? code + '.SH' : code + '.SZ')
            : code)
    window.open('/live-trading/' + sym, '_blank')
  }, [])

  if (loading && (!items || items.length === 0)) {
    return (
      <section className="rounded-2xl border border-border bg-card p-5">
        <RankingHeader exchange={exchange} onExchangeChange={onExchangeChange} />
        <div className="mt-6 flex items-center justify-center py-12 text-sm text-white/40">
          <span className="animate-pulse">加载精选榜单...</span>
        </div>
      </section>
    )
  }

  // Empty state
  if (!items || items.length === 0) {
    return (
      <section className="rounded-2xl border border-border bg-card p-5">
        <RankingHeader exchange={exchange} onExchangeChange={onExchangeChange} />
        <div className="mt-4 rounded-xl border border-dashed border-border px-4 py-8 text-center text-sm text-white/40">
          当前市场暂无明显机会标的，建议关注防御区选项。
        </div>
      </section>
    )
  }

  return (
    <section className="rounded-2xl border border-border bg-card p-5">
      <RankingHeader exchange={exchange} onExchangeChange={onExchangeChange} meta={meta} />

      {/* PC: compact list rows */}
      <div className="hidden md:block mt-4 space-y-1">
        {items.map((item) => (
          <RankRow key={item.code} item={item} onClick={() => handleItemClick(item.code, item.exchange)} />
        ))}
      </div>

      {/* Mobile: card grid */}
      <div className="md:hidden mt-4 grid gap-3 sm:grid-cols-2">
        {items.map((item) => (
          <RankCard key={item.code} item={item} onClick={() => handleItemClick(item.code, item.exchange)} />
        ))}
      </div>

      {/* Disclaimer */}
      <p className="mt-4 text-[11px] text-white/30 text-center">
        以上数据基于卧龙AI模型每日分析，仅供参考，不构成投资建议。
      </p>
    </section>
  )
}

// ── Sub-components ──

function RankingHeader({ exchange, onExchangeChange, meta }) {
  const tabs = [
    { key: 'ASHARE', label: 'A股精选' },
    { key: 'HKEX', label: '港股精选' },
  ]

  return (
    <div className="flex flex-wrap items-start justify-between gap-3">
      <div>
        <h3 className="text-base font-semibold text-white">
          ★ 卧龙AI精选
          <span className="ml-2 text-[11px] font-normal text-white/40">基于卧龙AI模型每日分析</span>
        </h3>
        {meta?.computed_at && (
          <div className="mt-1 text-xs text-white/50">
            机会区共 <span className="font-medium text-emerald-300">{meta.total_in_zone ?? '--'} 只</span>
            {' · 展示 Top '}
            <span className="font-medium">{meta.returned_count ?? '--'}</span>
          </div>
        )}
      </div>

      {/* Tab Switch */}
      <div className="flex items-center gap-1 rounded-lg bg-black/20 p-0.5">
        {tabs.map((tab) => (
          <button
            key={tab.key}
            type="button"
            onClick={() => onExchangeChange?.(tab.key)}
            className={`rounded-md px-3 py-1 text-xs font-medium transition ${
              exchange === tab.key
                ? 'bg-primary text-black'
                : 'text-white/55 hover:text-white/80 hover:bg-white/[0.05]'
            }`}
          >
            {tab.label}
          </button>
        ))}
      </div>
    </div>
  )
}

function RankRow({ item, onClick }) {
  const medal = getMedal(item.rank)
  const oppClass = item.opportunity >= 90 ? 'text-emerald-300' : item.opportunity >= 70 ? 'text-white' : 'text-white/60'
  const riskClass = item.risk < 30 ? 'text-emerald-300' : item.risk < 50 ? 'text-amber-300' : 'text-white/60'

  return (
    <div
      role="button"
      tabIndex={0}
      onKeyDown={(e) => e.key === 'Enter' && onClick()}
      onClick={onClick}
      className="group flex cursor-pointer items-center gap-3 rounded-xl px-3 py-2.5 transition hover:bg-white/[0.04]"
    >
      {/* Rank badge */}
      <span className={`flex w-7 shrink-0 items-center justify-center text-xs font-bold ${medal.className}`}>
        {medal.icon || item.rank}
      </span>

      {/* Name & Code */}
      <div className="min-w-0 flex-1">
        <div className="truncate text-sm font-semibold text-white group-hover:text-primary transition">
          {item.name}
        </div>
        <div className="text-[11px] text-white/35">
          {formatCode(item.code, item.exchange)} · {exchangeLabel(item.exchange)}
        </div>
      </div>

      {/* Scores */}
      <div className="flex shrink-0 items-center gap-4 text-xs tabular-nums">
        <div className="text-right">
          <div className={`${oppClass} font-semibold`}>{item.opportunity.toFixed(1)}</div>
          <div className="text-[10px] text-white/30">机会</div>
        </div>
        <div className="text-right w-10">
          <div className={`${riskClass}`}>{item.risk.toFixed(1)}</div>
          <div className="text-[10px] text-white/30">风险</div>
        </div>
        {/* Sub-scores bars */}
        <ScoreBar label="趋势" value={item.trend} max={100} width="w-16" />
        <ScoreBar label="资金" value={item.flow} max={100} width="w-14" />
        <ScoreBar label="修正" value={item.revision} max={100} width="w-14" />
      </div>

      {/* Arrow */}
      <span className="shrink-0 text-white/20 transition group-hover:text-primary group-hover:translate-x-0.5">→</span>
    </div>
  )
}

function RankCard({ item, onClick }) {
  const medal = getMedal(item.rank)

  return (
    <div
      role="button"
      tabIndex={0}
      onKeyDown={(e) => e.key === 'Enter' && onClick()}
      onClick={onClick}
      className="cursor-pointer rounded-xl border border-border/50 bg-black/15 p-3 transition hover:border-primary/40 active:scale-[0.98]"
    >
      <div className="flex items-center gap-2">
        <span className={`flex h-6 w-6 shrink-0 items-center justify-center rounded-full text-[11px] font-bold ${medal.className}`}>
          {medal.icon || item.rank}
        </span>
        <div className="min-w-0 flex-1">
          <div className="truncate text-sm font-semibold text-white">{item.name}</div>
          <div className="text-[11px] text-white/40">
            {formatCode(item.code, item.exchange)} · {exchangeLabel(item.exchange)}
          </div>
        </div>
      </div>

      <div className="mt-2.5 grid grid-cols-2 gap-x-3 gap-y-1 text-xs tabular-nums">
        <div>
          <span className="text-white/35">机会</span>
          <span className={`ml-1.5 font-semibold ${item.opportunity >= 90 ? 'text-emerald-300' : 'text-white'}`}>
            {item.opportunity.toFixed(1)}
          </span>
        </div>
        <div>
          <span className="text-white/35">风险</span>
          <span className={`ml-1.5 ${item.risk < 30 ? 'text-emerald-300' : 'text-white/60'}`}>
            {item.risk.toFixed(1)}
          </span>
        </div>
      </div>

      {/* Mini score bar */}
      <div className="mt-2 h-1.5 w-full overflow-hidden rounded-full bg-white/5">
        <div
          className="h-full rounded-full bg-gradient-to-r from-emerald-500 to-blue-500 transition-all"
          style={{ width: `${Math.min(100, item.opportunity)}%` }}
        />
      </div>

      <div className="mt-1.5 text-[10px] text-white/25">
        趋势{item.trend.toFixed(0)} · 资金{item.flow.toFixed(0)}
      </div>
    </div>
  )
}

function ScoreBar({ label, value, max, width }) {
  const pct = Math.min(100, Math.max(0, (value / max) * 100))
  return (
    <div className={`${width} hidden lg:block`}>
      <div className="mb-0.5 text-[10px] text-white/30">{label}</div>
      <div className="flex items-center gap-1.5">
        <div className="h-1 w-full overflow-hidden rounded-full bg-white/[0.06]">
          <div
            className="h-full rounded-full bg-gradient-to-r from-emerald-400/70 to-cyan-400/50"
            style={{ width: `${pct}%` }}
          />
        </div>
        <span className="w-5 text-right text-[10px] text-white/45 tabular-nums">{value.toFixed(0)}</span>
      </div>
    </div>
  )
}

// ── Pure helpers ──

function getMedal(rank) {
  if (rank === 1) return { icon: '🥇', className: '' }
  if (rank === 2) return { icon: '🥈', className: '' }
  if (rank === 3) return { icon: '🥉', className: '' }
  if (rank <= 10) return { icon: null, className: 'rounded-full bg-white/10 text-white text-[10px]' }
  return { icon: null, className: 'text-white/35 text-[10px]' }
}

function formatCode(code, exchange) {
  if (exchange === 'HKEX') return code.padStart(5, '0')
  return code
}

function exchangeLabel(ex) {
  const labels = { SSE: '沪市', SZSE: '深市', HKEX: '港股' }
  return labels[ex] || ex
}
