import { useCallback } from 'react'

/**
 * RankingPanel — 卧龙AI精选排行榜
 * A 股按精选评分展示，港股继续按机会区机会评分展示。
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
          当前市场暂无可展示精选标的，建议稍后再看。
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

function getPrimaryScoreLabel(exchange) {
  return exchange === 'HKEX' ? '机会评分' : '精选评分'
}

function getPrimaryScoreValue(item) {
  if (item?.exchange !== 'HKEX' && typeof item?.ranking_score === 'number' && Number.isFinite(item.ranking_score) && item.ranking_score > 0) {
    return item.ranking_score
  }
  if (typeof item?.opportunity === 'number' && Number.isFinite(item.opportunity)) {
    return item.opportunity
  }
  return 0
}

function RankingHeader({ exchange, onExchangeChange, meta }) {
  const tabs = [
    { key: 'ASHARE', label: 'A股精选' },
    { key: 'HKEX', label: '港股精选' },
  ]
  const metaSummary = buildRankingMetaSummary(meta, exchange)

  return (
    <div className="flex flex-wrap items-start justify-between gap-3">
      <div>
        <h3 className="text-base font-semibold text-white">★ 卧龙AI精选</h3>
        <div className="mt-1 text-[11px] text-white/35">基于卧龙AI模型每日分析</div>
        <div className="mt-1 max-w-2xl text-xs leading-5 text-white/45">
          {metaSummary}
        </div>
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
                : 'text-white/55 hover:bg-white/[0.05] hover:text-white/80'
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
  const primaryScore = getPrimaryScoreValue(item)
  const primaryScoreLabel = getPrimaryScoreLabel(item.exchange)
  const oppClass = primaryScore >= 90 ? 'text-emerald-300' : primaryScore >= 70 ? 'text-white/80' : 'text-white/55'
  const riskClass = item.risk < 30 ? 'text-emerald-300' : item.risk < 50 ? 'text-amber-300' : 'text-white/55'
  const returnClass = getReturnTextClass(item.return_pct)
  const consecutiveClass = getConsecutiveValueClass(item.consecutive_days)

  return (
    <div
      role="button"
      tabIndex={0}
      onKeyDown={(e) => e.key === 'Enter' && onClick()}
      onClick={onClick}
      className="group flex cursor-pointer items-center gap-4 rounded-xl px-3 py-3 transition hover:bg-white/[0.04]"
    >
      {/* Rank badge */}
      <span className={`flex w-7 shrink-0 items-center justify-center text-xs font-bold ${medal.className}`}>
        {medal.icon || item.rank}
      </span>

      {/* Identity */}
      <div className="min-w-0 flex-1">
        <div className="truncate text-[15px] font-semibold leading-5 text-white transition group-hover:text-primary">
          {item.name}
        </div>
        <div className="mt-0.5 text-[11px] text-white/35">
          {formatCode(item.code, item.exchange)} · {exchangeLabel(item.exchange)}
        </div>
      </div>

      {/* Result */}
      <div className="flex shrink-0 items-center gap-3 lg:gap-5">
        <div className="text-right">
          <div className={`text-[18px] font-semibold leading-none tabular-nums ${returnClass}`}>
            {formatReturnPctDisplay(item.return_pct)}
          </div>
          <div className="mt-1 text-[10px] text-white/30">上榜以来</div>
        </div>

        <div className="hidden min-[860px]:inline-flex items-center rounded-full bg-white/[0.05] px-2.5 py-1 text-[11px] font-medium">
          <span className="text-white/40">连续上榜</span>
          <span className={`ml-1.5 ${consecutiveClass}`}>{item.consecutive_days > 0 ? `${item.consecutive_days} 日` : '--'}</span>
        </div>
      </div>

      {/* Explanation */}
      <div className="hidden xl:flex shrink-0 items-center gap-4 text-xs tabular-nums">
        <MetricStat label={primaryScoreLabel} value={primaryScore.toFixed(1)} valueClass={`${oppClass} font-medium`} />
        <MetricStat label="风险评分" value={item.risk.toFixed(1)} valueClass={riskClass} />
        <ScoreBar label="趋势" value={item.trend} max={100} width="w-14" />
        <ScoreBar label="资金" value={item.flow} max={100} width="w-14" />
        <ScoreBar label="流动" value={item.liquidity ?? 50} max={100} width="w-16" amount={item.avg_amount_5d} />
      </div>

      <div className="hidden lg:flex xl:hidden shrink-0 flex-col items-end gap-0.5 text-[10px] text-white/30 tabular-nums">
        <span>{`${primaryScoreLabel} ${primaryScore.toFixed(1)}`}</span>
        <span>{`风险评分 ${item.risk.toFixed(1)}`}</span>
      </div>

      {/* Arrow */}
      <span className="shrink-0 text-white/20 transition group-hover:translate-x-0.5 group-hover:text-primary">→</span>
    </div>
  )
}

function RankCard({ item, onClick }) {
  const medal = getMedal(item.rank)
  const primaryScore = getPrimaryScoreValue(item)
  const primaryScoreLabel = getPrimaryScoreLabel(item.exchange)
  const returnClass = getReturnTextClass(item.return_pct)
  const consecutiveClass = getConsecutiveValueClass(item.consecutive_days)

  return (
    <div
      role="button"
      tabIndex={0}
      onKeyDown={(e) => e.key === 'Enter' && onClick()}
      onClick={onClick}
      className="cursor-pointer rounded-xl border border-border/50 bg-black/15 p-3 transition hover:border-primary/40 active:scale-[0.98]"
    >
      <div className="flex items-start justify-between gap-3">
        <div className="flex min-w-0 flex-1 items-start gap-2">
          <span className={`flex h-6 w-6 shrink-0 items-center justify-center rounded-full text-[11px] font-bold ${medal.className}`}>
            {medal.icon || item.rank}
          </span>
          <div className="min-w-0 flex-1">
            <div className="truncate text-sm font-semibold text-white">{item.name}</div>
            <div className="mt-0.5 text-[11px] text-white/40">
              {formatCode(item.code, item.exchange)} · {exchangeLabel(item.exchange)}
            </div>
          </div>
        </div>

        <div className="shrink-0 text-right">
          <div className={`text-base font-semibold leading-none tabular-nums ${returnClass}`}>
            {formatReturnPctDisplay(item.return_pct)}
          </div>
          <div className="mt-1 text-[10px] text-white/30">上榜以来</div>
        </div>
      </div>

      <div className="mt-3 flex items-center justify-between gap-2">
        <div className="inline-flex max-w-full items-center rounded-full bg-white/[0.05] px-2 py-1 text-[10px] font-medium">
          <span className="text-white/40">连续上榜</span>
          <span className={`ml-1.5 ${consecutiveClass}`}>{item.consecutive_days > 0 ? `${item.consecutive_days} 日` : '--'}</span>
        </div>
        <div className="text-[10px] text-white/30 tabular-nums">
          {`${primaryScoreLabel} ${primaryScore.toFixed(1)} · 风险评分 ${item.risk.toFixed(1)}`}
        </div>
      </div>

      <div className="mt-3 rounded-lg bg-white/[0.02] px-3 py-2">
        <div className="flex flex-wrap items-center gap-x-3 gap-y-1 text-[10px] text-white/35 tabular-nums">
          <span>{`趋势 ${item.trend.toFixed(0)}`}</span>
          <span>{`资金 ${item.flow.toFixed(0)}`}</span>
          <span title={item.avg_amount_5d ? `近5日均成交额 ${formatAmount(item.avg_amount_5d)}` : undefined}>
            {`流动 ${item.avg_amount_5d ? formatAmount(item.avg_amount_5d) : '--'}`}
          </span>
        </div>

        <div className="mt-2 h-1 overflow-hidden rounded-full bg-white/[0.05]">
          <div
            className="h-full rounded-full bg-gradient-to-r from-emerald-400/60 to-cyan-400/40 transition-all"
            style={{ width: `${Math.min(100, primaryScore)}%` }}
          />
        </div>
      </div>
    </div>
  )
}

function MetricStat({ label, value, valueClass }) {
  return (
    <div className="text-right">
      <div className={`text-[12px] tabular-nums ${valueClass}`}>{value}</div>
      <div className="mt-0.5 text-[10px] text-white/25">{label}</div>
    </div>
  )
}

function ScoreBar({ label, value, max, width, amount = null }) {
  const pct = Math.min(100, Math.max(0, (value / max) * 100))
  return (
    <div className={`${width} hidden xl:block`}>
      <div className="mb-0.5 text-[10px] text-white/25">{label}</div>
      <div className="flex items-center gap-1.5">
        <div className="h-1 w-full overflow-hidden rounded-full bg-white/[0.05]">
          <div
            className="h-full rounded-full bg-gradient-to-r from-emerald-400/55 to-cyan-400/35"
            style={{ width: `${pct}%` }}
          />
        </div>
        {amount != null && amount > 0 ? (
          <span className="text-[9px] text-white/30 tabular-nums" title={`近5日均成交额 ${formatAmount(amount)}`}>
            {formatAmount(amount)}
          </span>
        ) : (
          <span className="w-5 text-right text-[9px] text-white/35 tabular-nums">{value.toFixed(0)}</span>
        )}
      </div>
    </div>
  )
}

function formatAmount(val) {
  if (!val || val <= 0) return '--'
  if (val >= 10000) return `${(val / 10000).toFixed(1)}亿`
  return `${val.toFixed(0)}万`
}

function formatMetaDateTime(value) {
  if (!value) return '--'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return date.toLocaleString('zh-CN', { hour12: false })
}

function buildRankingMetaSummary(meta, currentExchange = 'ASHARE') {
  const exchange = meta?.exchange || currentExchange
  const parts = [exchange === 'HKEX' ? '港股榜单来自机会区' : 'A股榜单按精选评分排序']
  if (meta?.computed_at) {
    parts.push(`数据日期：${formatMetaDateTime(meta.computed_at)}`)
  }
  if (meta?.returned_count != null) {
    parts.push(`当前展示 TOP${meta.returned_count} 只`)
  }
  return parts.join(' · ')
}

function hasReturnPct(value) {
  return typeof value === 'number' && Number.isFinite(value)
}

function formatReturnPctDisplay(value) {
  if (!hasReturnPct(value)) return '--'
  const prefix = value > 0 ? '+' : ''
  return `${prefix}${value.toFixed(1)}%`
}

function getReturnTextClass(value) {
  if (!hasReturnPct(value)) return 'text-white/25'
  return value >= 0 ? 'text-red-400' : 'text-green-400'
}

function getConsecutiveValueClass(value) {
  if (!value || value <= 0) return 'text-white/25'
  return value >= 7 ? 'text-emerald-300' : 'text-white/75'
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
