import { useEffect, useMemo, useState } from 'react'
import { LabelWithInfo } from './InfoTip'
import {
  ATTRIBUTION_RANGE_OPTIONS,
  attributionToneClass,
  buildAttributionDetailRequestKeys,
  buildAttributionHeroBadges,
  buildAttributionWaterfallSeries,
  createAttributionDetailSectionsState,
  formatAttributionMoney,
  formatAttributionPercent,
  pickAttributionMarketSnapshot,
  pickAttributionSectorHighlights,
  pickAttributionStockHighlights,
  pickAttributionTradingHighlights,
  resolveAttributionActiveScope,
} from '../lib/portfolio-attribution'

const ATTRIBUTION_TIPS = {
  section: '绩效归因把组合收益拆成“市场行情、选股超额、调仓贡献、手续费”几部分，帮助你知道这段时间到底赚在趋势、赚在选股，还是亏在频繁交易。',
  waterfall: '瀑布图会把收益拆成市场、选股、调仓和手续费几个来源，帮助你一眼看懂结果是怎么形成的。',
  stocks: '只保留最关键的正贡献和负贡献股票，帮助你快速定位真正拉动或拖累组合的标的。',
  trading: '关键交易只展示最有帮助和最拖后腿的几笔操作，避免被完整时间线淹没。',
  market: '市场对比用来判断这段时间表现更多来自大盘，还是来自自己的配置与选股。',
  sectors: '行业归因帮助你看清收益或拖累主要集中在哪些行业。',
  benchmark: 'A股当前默认以上证指数为基准，港股默认以恒生指数为基准。',
}

const EMPTY_DETAIL_LOADING = {
  stocks: false,
  trading: false,
  market: false,
  sectors: false,
}

const EMPTY_DETAIL_ERROR = {
  stocks: '',
  trading: '',
  market: '',
  sectors: '',
}

function formatComputedAt(value) {
  if (!value) return '--'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return '--'
  return date.toLocaleString('zh-CN', { hour12: false })
}

function formatEventTypeLabel(value) {
  switch (String(value || '').toLowerCase()) {
    case 'buy':
      return '买入'
    case 'sell':
      return '卖出'
    case 'adjust_avg_cost':
      return '调均价'
    default:
      return value || '交易'
  }
}

function EmptyState({ text, compact = false }) {
  return (
    <div className={`rounded-2xl border border-dashed border-white/10 bg-black/20 px-4 text-center text-sm text-white/40 ${compact ? 'py-5' : 'py-10'}`}>
      {text}
    </div>
  )
}

function HeroSkeleton() {
  return (
    <div className="space-y-4 rounded-3xl border border-white/10 bg-white/[0.03] p-4 sm:p-6">
      <div className="flex flex-wrap items-center gap-2">
        {Array.from({ length: 4 }).map((_, index) => (
          <div key={index} className="h-8 w-16 animate-pulse rounded-full bg-white/[0.06]" />
        ))}
      </div>
      <div className="h-5 w-3/4 animate-pulse rounded bg-white/[0.06]" />
      <div className="h-64 animate-pulse rounded-3xl bg-white/[0.04]" />
      <div className="grid gap-3 sm:grid-cols-3">
        {Array.from({ length: 3 }).map((_, index) => (
          <div key={index} className="h-20 animate-pulse rounded-2xl bg-white/[0.04]" />
        ))}
      </div>
    </div>
  )
}

function SectionCard({ title, tooltip, children, action, compact = false }) {
  return (
    <div className={`rounded-2xl border border-white/10 bg-white/[0.03] ${compact ? 'p-3.5' : 'p-4 sm:p-5'}`}>
      <div className="mb-3 flex items-start justify-between gap-3">
        <h4 className="text-sm font-semibold text-white/84">
          <LabelWithInfo label={title} tooltip={tooltip} />
        </h4>
        {action ? <div className="shrink-0">{action}</div> : null}
      </div>
      {children}
    </div>
  )
}

function MetricBadge({ badge }) {
  return (
    <div className="rounded-2xl border border-white/10 bg-black/20 px-3.5 py-3">
      <div className="text-[11px] text-white/40">{badge.label}</div>
      <div className={`mt-1 text-sm font-semibold ${attributionToneClass(badge.toneValue)}`}>{badge.value}</div>
      {badge.subValue ? <div className="mt-1 text-[11px] text-white/36">{badge.subValue}</div> : null}
    </div>
  )
}

function DetailListSkeleton({ rows = 3 }) {
  return (
    <div className="space-y-2.5">
      {Array.from({ length: rows }).map((_, index) => (
        <div key={index} className="h-16 animate-pulse rounded-2xl bg-white/[0.04]" />
      ))}
    </div>
  )
}

function RetryInline({ message, onRetry }) {
  return (
    <div className="rounded-2xl border border-amber-500/20 bg-amber-500/[0.06] px-3.5 py-3 text-xs text-amber-100">
      <div>{message}</div>
      {onRetry ? (
        <button
          type="button"
          onClick={onRetry}
          className="mt-2 rounded-full border border-amber-400/30 px-3 py-1 text-[11px] font-medium text-amber-100 transition hover:bg-amber-400/10"
        >
          重新加载
        </button>
      ) : null}
    </div>
  )
}

function WaterfallChart({ group }) {
  const series = useMemo(() => buildAttributionWaterfallSeries(group), [group])

  if (!series.length) {
    return <EmptyState text="当前范围暂无可展示的收益拆解。" compact />
  }

  const width = 720
  const height = 260
  const top = 18
  const bottom = 68
  const left = 18
  const right = 18
  const plotWidth = width - left - right
  const plotHeight = height - top - bottom
  const stepWidth = plotWidth / series.length
  const values = series.flatMap((item) => [item.start, item.end, 0])
  const maxValue = Math.max(...values)
  const minValue = Math.min(...values)
  const range = Math.max(maxValue - minValue, 1)
  const zeroY = top + ((maxValue - 0) / range) * plotHeight
  const yFor = (value) => top + ((maxValue - value) / range) * plotHeight

  return (
    <div>
      <svg viewBox={`0 0 ${width} ${height}`} className="h-64 w-full rounded-3xl border border-white/10 bg-black/25 p-2">
        <line x1={left} x2={width - right} y1={zeroY} y2={zeroY} stroke="rgba(255,255,255,0.12)" strokeDasharray="4 4" />

        {series.map((item, index) => {
          const x = left + index * stepWidth + stepWidth * 0.18
          const barWidth = stepWidth * 0.64
          const startValue = item.isTotal ? 0 : item.start
          const endValue = item.end
          const high = Math.max(startValue, endValue)
          const low = Math.min(startValue, endValue)
          const y = yFor(high)
          const barHeight = Math.max(yFor(low) - y, 4)
          const next = series[index + 1]
          const connectorY = yFor(endValue)
          const fill = item.isTotal ? '#f8fafc' : item.amount >= 0 ? '#fb7185' : '#34d399'

          return (
            <g key={`${group?.scope || 'ALL'}-${item.key || index}`}>
              {!item.isTotal && next ? (
                <line
                  x1={x + barWidth}
                  x2={left + (index + 1) * stepWidth + stepWidth * 0.18}
                  y1={connectorY}
                  y2={connectorY}
                  stroke="rgba(255,255,255,0.18)"
                  strokeDasharray="4 4"
                />
              ) : null}
              <rect x={x} y={y} width={barWidth} height={barHeight} rx="12" fill={fill} opacity={item.isTotal ? 0.92 : 0.88} />
              <text x={x + barWidth / 2} y={Math.max(y - 8, 12)} textAnchor="middle" fontSize="10" fill="rgba(255,255,255,0.88)">
                {formatAttributionMoney(item.amount, group?.currency_symbol, { compact: true })}
              </text>
              <text x={x + barWidth / 2} y={height - 28} textAnchor="middle" fontSize="11" fill="rgba(255,255,255,0.72)">
                {item.label}
              </text>
            </g>
          )
        })}
      </svg>
      <div className="mt-3 grid gap-2 sm:grid-cols-5">
        {series.map((item) => (
          <div key={`${group?.scope || 'ALL'}-legend-${item.key}`} className="rounded-2xl border border-white/8 bg-white/[0.03] px-3 py-2.5">
            <div className="text-[11px] text-white/42">{item.label}</div>
            <div className={`mt-1 text-sm font-medium ${attributionToneClass(item.amount, item.isTotal ? 'text-white' : 'text-white/82')}`}>
              {formatAttributionMoney(item.amount, group?.currency_symbol, { compact: true })}
            </div>
          </div>
        ))}
      </div>
    </div>
  )
}

function ScopeSwitch({ scopes, activeScope, onChange }) {
  if (!Array.isArray(scopes) || scopes.length <= 1) return null

  return (
    <div className="inline-flex items-center gap-1 rounded-full border border-white/10 bg-black/25 p-1">
      {scopes.map((scope) => (
        <button
          key={scope.scope}
          type="button"
          onClick={() => onChange(scope.scope)}
          className={`rounded-full px-3 py-1.5 text-xs font-medium transition ${
            activeScope === scope.scope
              ? 'bg-white text-slate-950'
              : 'text-white/48 hover:bg-white/[0.06] hover:text-white/84'
          }`}
        >
          {scope.label}
        </button>
      ))}
    </div>
  )
}

function StockHighlightList({ items, currencySymbol, emptyText }) {
  if (!items?.length) {
    return <EmptyState text={emptyText} compact />
  }

  return (
    <div className="space-y-2.5">
      {items.map((item) => (
        <div key={item.symbol} className="rounded-2xl bg-black/20 px-3.5 py-3">
          <div className="flex items-start justify-between gap-3">
            <div className="min-w-0">
              <div className="flex items-center gap-2">
                <span className="font-mono text-sm text-white/86">{item.symbol}</span>
                <span className="truncate text-sm text-white/68">{item.name}</span>
              </div>
              <div className="mt-1 text-[11px] text-white/36">{item.driver_label || '归因解释待补充'}</div>
            </div>
            <div className="text-right">
              <div className={`text-sm font-semibold ${attributionToneClass(item.total_pnl_amount)}`}>
                {formatAttributionMoney(item.total_pnl_amount, currencySymbol, { compact: true })}
              </div>
              <div className="mt-1 text-[11px] text-white/36">贡献 {formatAttributionPercent(item.contribution_ratio, 1)}</div>
            </div>
          </div>
        </div>
      ))}
    </div>
  )
}

function TradeHighlightList({ items, currencySymbol, emptyText }) {
  if (!items?.length) {
    return <EmptyState text={emptyText} compact />
  }

  return (
    <div className="space-y-2.5">
      {items.map((item) => (
        <div key={item.event_id} className="rounded-2xl bg-black/20 px-3.5 py-3">
          <div className="flex items-start justify-between gap-3">
            <div className="min-w-0">
              <div className="flex items-center gap-2 text-sm text-white/84">
                <span>{item.trade_date}</span>
                <span className="rounded-full border border-white/10 px-2 py-0.5 text-[10px] text-white/44">{formatEventTypeLabel(item.event_type)}</span>
                <span className="font-mono text-white/52">{item.symbol}</span>
              </div>
              <div className="mt-1 text-[11px] text-white/36">{item.note || item.name || '关键交易'}</div>
            </div>
            <div className="text-right">
              <div className={`text-sm font-semibold ${attributionToneClass(item.timing_effect_amount)}`}>
                {formatAttributionMoney(item.timing_effect_amount, currencySymbol, { compact: true })}
              </div>
              <div className="mt-1 text-[11px] text-white/36">已实现 {formatAttributionMoney(item.realized_pnl_amount, currencySymbol, { compact: true })}</div>
            </div>
          </div>
        </div>
      ))}
    </div>
  )
}

function MarketSeriesChart({ series = [] }) {
  const width = 360
  const height = 150
  const padding = 12

  if (!series.length) {
    return <EmptyState text="当前没有可绘制的市场对比曲线。" compact />
  }

  const allValues = series.flatMap((item) => [item.portfolio_nav, item.benchmark_nav]).filter((value) => typeof value === 'number' && Number.isFinite(value))
  const maxValue = Math.max(...allValues, 1)
  const minValue = Math.min(...allValues, 0.8)
  const valueRange = Math.max(maxValue - minValue, 0.0001)
  const xStep = series.length > 1 ? (width - padding * 2) / (series.length - 1) : 0

  const buildPath = (field) => series.map((item, index) => {
    const x = padding + index * xStep
    const y = height - padding - (((item?.[field] || 0) - minValue) / valueRange) * (height - padding * 2)
    return `${index === 0 ? 'M' : 'L'} ${x.toFixed(1)} ${y.toFixed(1)}`
  }).join(' ')

  return (
    <div>
      <svg viewBox={`0 0 ${width} ${height}`} className="h-36 w-full rounded-2xl border border-white/8 bg-black/20">
        <path d={buildPath('portfolio_nav')} fill="none" stroke="#fb7185" strokeWidth="2.4" strokeLinecap="round" strokeLinejoin="round" />
        <path d={buildPath('benchmark_nav')} fill="none" stroke="#60a5fa" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round" strokeDasharray="5 4" />
      </svg>
      <div className="mt-2 flex flex-wrap items-center gap-4 text-[11px] text-white/42">
        <span className="inline-flex items-center gap-1"><span className="h-2 w-2 rounded-full bg-rose-400" />组合净值</span>
        <span className="inline-flex items-center gap-1"><span className="h-2 w-2 rounded-full bg-sky-400" />基准净值</span>
      </div>
    </div>
  )
}

function ExpandButton({ open, label, onClick }) {
  return (
    <button
      type="button"
      onClick={onClick}
      className="inline-flex items-center gap-2 rounded-full border border-white/10 px-3 py-1.5 text-[11px] font-medium text-white/70 transition hover:border-white/20 hover:bg-white/[0.05] hover:text-white"
    >
      <span>{label}</span>
      <span>{open ? '收起' : '展开'}</span>
    </button>
  )
}

function DetailContent({
  activeScope,
  stocks,
  trading,
  market,
  sectors,
  detailLoading,
  detailError,
  marketExpanded,
  sectorExpanded,
  onRetry,
  onToggleMarket,
  onToggleSectors,
  onClose,
}) {
  const stockHighlights = useMemo(() => pickAttributionStockHighlights(stocks, activeScope), [stocks, activeScope])
  const tradingHighlights = useMemo(() => pickAttributionTradingHighlights(trading, activeScope), [trading, activeScope])
  const marketSnapshot = useMemo(() => pickAttributionMarketSnapshot(market, activeScope), [market, activeScope])
  const sectorHighlights = useMemo(() => pickAttributionSectorHighlights(sectors, activeScope), [sectors, activeScope])
  const currencySymbol = stockHighlights.currencySymbol || tradingHighlights.group?.currency_symbol || marketSnapshot?.currency_symbol || sectorHighlights.group?.currency_symbol || ''

  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between gap-3">
        <div>
          <div className="text-sm font-semibold text-white/86">详细归因</div>
          <div className="mt-1 text-[11px] text-white/38">关键股票、关键交易、市场对比和行业归因按同一层连续展开，方便直接顺着看完。</div>
        </div>
        {onClose ? (
          <button
            type="button"
            onClick={onClose}
            className="rounded-full border border-white/10 px-3 py-1.5 text-[11px] text-white/60 transition hover:bg-white/[0.05] hover:text-white/88"
          >
            收起详情
          </button>
        ) : null}
      </div>

      <div className="grid gap-3 lg:grid-cols-2">
        <SectionCard title="关键股票" tooltip={ATTRIBUTION_TIPS.stocks}>
          {detailLoading.stocks ? (
            <DetailListSkeleton />
          ) : detailError.stocks ? (
            <RetryInline message={detailError.stocks} onRetry={() => onRetry('stocks')} />
          ) : (
            <div className="grid gap-3 lg:grid-cols-2">
              <div>
                <div className="mb-2 text-[11px] text-white/42">最大正贡献</div>
                <StockHighlightList items={stockHighlights.positive} currencySymbol={currencySymbol} emptyText="当前没有明显的正贡献股票。" />
              </div>
              <div>
                <div className="mb-2 text-[11px] text-white/42">最大负贡献</div>
                <StockHighlightList items={stockHighlights.negative} currencySymbol={currencySymbol} emptyText="当前没有明显的负贡献股票。" />
              </div>
            </div>
          )}
        </SectionCard>

        <SectionCard title="关键交易" tooltip={ATTRIBUTION_TIPS.trading}>
          {detailLoading.trading ? (
            <DetailListSkeleton />
          ) : detailError.trading ? (
            <RetryInline message={detailError.trading} onRetry={() => onRetry('trading')} />
          ) : (
            <div className="grid gap-3 lg:grid-cols-2">
              <div>
                <div className="mb-2 text-[11px] text-white/42">最有效交易</div>
                <TradeHighlightList items={tradingHighlights.positive} currencySymbol={currencySymbol} emptyText="当前没有明显加分的交易。" />
              </div>
              <div>
                <div className="mb-2 text-[11px] text-white/42">最拖后腿交易</div>
                <TradeHighlightList items={tradingHighlights.negative} currencySymbol={currencySymbol} emptyText="当前没有明显拖累的交易。" />
              </div>
            </div>
          )}
        </SectionCard>
      </div>

      <SectionCard
        title="市场对比"
        tooltip={ATTRIBUTION_TIPS.market}
        action={<ExpandButton open={marketExpanded} label="组合 vs 基准" onClick={onToggleMarket} />}
      >
        {marketExpanded ? (detailLoading.market ? (
          <DetailListSkeleton rows={2} />
        ) : detailError.market ? (
          <RetryInline message={detailError.market} onRetry={() => onRetry('market')} />
        ) : marketSnapshot ? (
          <div className="space-y-4">
            <div className="grid gap-2.5 sm:grid-cols-3">
              <div className="rounded-2xl bg-black/20 px-3.5 py-3">
                <div className="text-[11px] text-white/40">组合收益率</div>
                <div className={`mt-1 text-sm font-semibold ${attributionToneClass(marketSnapshot.portfolio_return_pct)}`}>
                  {formatAttributionPercent(marketSnapshot.portfolio_return_pct, 1)}
                </div>
              </div>
              <div className="rounded-2xl bg-black/20 px-3.5 py-3">
                <div className="text-[11px] text-white/40">基准收益率</div>
                <div className={`mt-1 text-sm font-semibold ${attributionToneClass(marketSnapshot.benchmark_return_pct)}`}>
                  {formatAttributionPercent(marketSnapshot.benchmark_return_pct, 1)}
                </div>
              </div>
              <div className="rounded-2xl bg-black/20 px-3.5 py-3">
                <div className="text-[11px] text-white/40">超额收益率</div>
                <div className={`mt-1 text-sm font-semibold ${attributionToneClass(marketSnapshot.excess_return_pct)}`}>
                  {formatAttributionPercent(marketSnapshot.excess_return_pct, 1)}
                </div>
              </div>
            </div>
            <div className="text-[11px] text-white/38">
              <LabelWithInfo label={`基准：${marketSnapshot.benchmark_name || marketSnapshot.benchmark_code || '--'}`} tooltip={ATTRIBUTION_TIPS.benchmark} />
            </div>
            <MarketSeriesChart series={marketSnapshot.series} />
          </div>
        ) : (
          <EmptyState text="当前范围暂无市场对比数据。" compact />
        )) : null}
      </SectionCard>

      <SectionCard
        title="行业归因"
        tooltip={ATTRIBUTION_TIPS.sectors}
        action={<ExpandButton open={sectorExpanded} label="查看行业" onClick={onToggleSectors} />}
      >
        {sectorExpanded ? (detailLoading.sectors ? (
          <DetailListSkeleton rows={2} />
        ) : detailError.sectors ? (
          <RetryInline message={detailError.sectors} onRetry={() => onRetry('sectors')} />
        ) : sectorHighlights.group ? (
          <div className="grid gap-3 lg:grid-cols-2">
            <div>
              <div className="mb-2 text-[11px] text-white/42">正贡献行业</div>
              <StockHighlightList
                items={sectorHighlights.positive.map((item) => ({
                  symbol: item.sector_name || '未分类',
                  name: `${item.stock_count || 0} 只股票`,
                  driver_label: item.driver_label,
                  total_pnl_amount: item.total_pnl_amount,
                  contribution_ratio: item.contribution_ratio,
                }))}
                currencySymbol={sectorHighlights.group.currency_symbol}
                emptyText="当前没有明显的正贡献行业。"
              />
            </div>
            <div>
              <div className="mb-2 text-[11px] text-white/42">负贡献行业</div>
              <StockHighlightList
                items={sectorHighlights.negative.map((item) => ({
                  symbol: item.sector_name || '未分类',
                  name: `${item.stock_count || 0} 只股票`,
                  driver_label: item.driver_label,
                  total_pnl_amount: item.total_pnl_amount,
                  contribution_ratio: item.contribution_ratio,
                }))}
                currencySymbol={sectorHighlights.group.currency_symbol}
                emptyText="当前没有明显的负贡献行业。"
              />
            </div>
          </div>
        ) : (
          <EmptyState text="当前范围暂无行业归因数据。" compact />
        )) : null}
      </SectionCard>
    </div>
  )
}

export default function PortfolioAttributionSection({
  loading = false,
  error = '',
  range = '30D',
  onRangeChange,
  summary,
  stocks,
  sectors,
  trading,
  market,
  detailLoading = EMPTY_DETAIL_LOADING,
  detailError = EMPTY_DETAIL_ERROR,
  onRequestDetails,
}) {
  const defaultDetailSections = useMemo(() => createAttributionDetailSectionsState(), [])
  const [detailOpen, setDetailOpen] = useState(false)
  const [marketExpanded, setMarketExpanded] = useState(defaultDetailSections.marketExpanded)
  const [sectorExpanded, setSectorExpanded] = useState(defaultDetailSections.sectorExpanded)
  const [activeScope, setActiveScope] = useState(null)

  const baseHero = useMemo(() => buildAttributionHeroBadges(summary), [summary])
  const hero = useMemo(() => buildAttributionHeroBadges(summary, activeScope || baseHero.activeScope), [summary, activeScope, baseHero.activeScope])
  const hasData = Boolean(summary?.has_data && hero.activeGroup)

  useEffect(() => {
    setActiveScope((prev) => resolveAttributionActiveScope(baseHero.availableScopes, prev, baseHero.activeScope || null))
  }, [baseHero.availableScopes, baseHero.activeScope, summary?.computed_at, summary?.start_date, summary?.end_date])

  useEffect(() => {
    if (detailOpen) return
    setMarketExpanded(defaultDetailSections.marketExpanded)
    setSectorExpanded(defaultDetailSections.sectorExpanded)
  }, [defaultDetailSections.marketExpanded, defaultDetailSections.sectorExpanded, detailOpen])

  useEffect(() => {
    const keys = buildAttributionDetailRequestKeys({
      detailOpen,
      marketExpanded,
      sectorExpanded,
    })
    if (!keys.length) return
    onRequestDetails?.(keys)
  }, [detailOpen, marketExpanded, onRequestDetails, sectorExpanded])

  const handleRetry = (key) => {
    onRequestDetails?.([key])
  }

  if (loading && !summary) {
    return <HeroSkeleton />
  }

  if (error && !summary) {
    return <EmptyState text={error} />
  }

  if (!hasData) {
    return <EmptyState text={summary?.empty_reason || '当前范围暂无可展示的绩效归因数据。'} />
  }

  return (
    <section className="space-y-4">
      <div className="rounded-3xl border border-white/10 bg-white/[0.03] p-4 sm:p-6">
        <div className="flex flex-col gap-4">
          <div className="flex flex-wrap items-start justify-between gap-3">
            <div>
              <h3 className="text-sm font-semibold text-white/86">
                <LabelWithInfo label="绩效归因分析" tooltip={ATTRIBUTION_TIPS.section} />
              </h3>
              <p className="mt-1 text-xs text-white/38">
                {summary?.mixed_currency ? 'A/H 混仓按市场分块返回，不做跨币种硬合并；默认一次只看一个市场。' : '默认只给一个结论和一张主图，帮你先看懂结果，再决定要不要深挖。'}
              </p>
            </div>
            <div className="text-[11px] text-white/34">计算于 {formatComputedAt(summary?.computed_at)}</div>
          </div>

          <div className="flex flex-col gap-3 xl:flex-row xl:items-center xl:justify-between">
            <div className="inline-flex flex-wrap items-center gap-2 rounded-2xl border border-white/10 bg-black/20 p-1">
              {ATTRIBUTION_RANGE_OPTIONS.map((option) => (
                <button
                  key={option.value}
                  type="button"
                  onClick={() => onRangeChange?.(option.value)}
                  className={`rounded-xl px-3 py-1.5 text-xs font-medium transition ${
                    range === option.value
                      ? 'border border-primary/35 bg-primary/20 text-primary'
                      : 'text-white/48 hover:bg-white/[0.05] hover:text-white/82'
                  }`}
                >
                  {option.label}
                </button>
              ))}
            </div>

            <ScopeSwitch scopes={hero.availableScopes} activeScope={hero.activeScope} onChange={setActiveScope} />
          </div>

          <div className="rounded-3xl border border-white/10 bg-black/20 p-4 sm:p-5">
            <div className="flex flex-col gap-4 lg:flex-row lg:items-start lg:justify-between">
              <div className="max-w-2xl">
                <div className="text-lg font-semibold leading-8 text-white/92">{hero.headline || '这段时间的收益拆解结果如下。'}</div>
                <div className="mt-2 text-sm text-white/42">默认只保留主结论和主图，详情只在你主动展开时显示。</div>
              </div>
              <button
                type="button"
                onClick={() => setDetailOpen((prev) => !prev)}
                className="inline-flex items-center justify-center rounded-full border border-white/10 px-4 py-2 text-sm font-medium text-white/78 transition hover:border-white/20 hover:bg-white/[0.06] hover:text-white"
              >
                {detailOpen ? '收起详情' : '查看详细归因'}
              </button>
            </div>

            <div className="mt-5">
              <WaterfallChart group={hero.activeGroup} />
            </div>

            <div className="mt-4 grid gap-3 sm:grid-cols-3">
              {hero.badges.map((badge) => <MetricBadge key={badge.key} badge={badge} />)}
            </div>
          </div>
        </div>
      </div>

      {detailOpen ? (
        <div className="hidden sm:block">
          <div className="rounded-3xl border border-white/10 bg-white/[0.02] p-4 sm:p-5">
            <DetailContent
              activeScope={hero.activeScope}
              stocks={stocks}
              trading={trading}
              market={market}
              sectors={sectors}
              detailLoading={detailLoading}
              detailError={detailError}
              marketExpanded={marketExpanded}
              sectorExpanded={sectorExpanded}
              onRetry={handleRetry}
              onToggleMarket={() => setMarketExpanded((prev) => !prev)}
              onToggleSectors={() => setSectorExpanded((prev) => !prev)}
              onClose={() => setDetailOpen(false)}
            />
          </div>
        </div>
      ) : null}

      {detailOpen ? (
        <div className="sm:hidden">
          <div className="fixed inset-0 z-40 bg-black/60 backdrop-blur-[2px]" onClick={() => setDetailOpen(false)} />
          <div className="fixed inset-x-0 bottom-0 z-50 max-h-[78vh] overflow-y-auto rounded-t-3xl border border-white/10 bg-[#0f1115] px-4 pb-6 pt-3">
            <div className="mx-auto mb-4 h-1.5 w-12 rounded-full bg-white/15" />
            <DetailContent
              activeScope={hero.activeScope}
              stocks={stocks}
              trading={trading}
              market={market}
              sectors={sectors}
              detailLoading={detailLoading}
              detailError={detailError}
              marketExpanded={marketExpanded}
              sectorExpanded={sectorExpanded}
              onRetry={handleRetry}
              onToggleMarket={() => setMarketExpanded((prev) => !prev)}
              onToggleSectors={() => setSectorExpanded((prev) => !prev)}
              onClose={() => setDetailOpen(false)}
            />
          </div>
        </div>
      ) : null}
    </section>
  )
}
