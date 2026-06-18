import { useEffect, useMemo, useRef, useState } from 'react'

import { useTheme } from '../lib/theme-context'
import { LabelWithInfo } from './InfoTip'
import { formatCloseDateLabel } from '../lib/trade-date-label'
import {
  buildRankingPortfolioDetailHref,
  buildRankingPortfolioChartSeriesData,
  formatRankingPortfolioCode,
  formatRankingPortfolioDate,
  formatRankingPortfolioDateTime,
  formatRankingPortfolioPercent,
  formatRankingPortfolioReferencePrice,
  formatRankingPortfolioWeightChange,
  getRankingPortfolioRebalanceActionLabel,
  getRankingPortfolioPerformanceClass,
} from '../lib/ranking-portfolio'

const CHART_COLORS = {
  portfolio: '#f59e0b',
  baseline: 'rgba(148,163,184,0.35)',
  grid: 'rgba(148,163,184,0.10)',
  border: 'rgba(148,163,184,0.20)',
  textDark: 'rgba(255,255,255,0.72)',
  textLight: 'rgba(30,41,59,0.78)',
}

const MARKET_CONFIG = {
  ASHARE: {
    key: 'ASHARE',
    label: 'A股',
    sectionTitle: 'A股组合追踪',
    accentClass: 'text-amber-700 dark:text-amber-200',
    badgeClass: 'border-amber-300/40 bg-amber-100 text-amber-800 dark:border-amber-300/20 dark:bg-amber-500/10 dark:text-amber-200',
    sectionClass: 'border-amber-300/40 bg-[linear-gradient(180deg,rgba(251,191,36,0.12),rgba(251,191,36,0.02))] dark:border-amber-300/20 dark:bg-[linear-gradient(180deg,rgba(245,158,11,0.14),rgba(245,158,11,0.03))]',
    dividerClass: 'border-amber-300/30 dark:border-amber-300/15',
    glowClass: 'shadow-[0_0_0_1px_rgba(245,158,11,0.06)]',
    chartTintClass: 'bg-[radial-gradient(circle_at_top_left,rgba(245,158,11,0.14),transparent_45%),linear-gradient(180deg,rgba(255,255,255,0.03),rgba(255,255,255,0.01))]',
  },
  HKEX: {
    key: 'HKEX',
    label: '港股',
    sectionTitle: '港股组合追踪',
    accentClass: 'text-sky-700 dark:text-sky-200',
    badgeClass: 'border-sky-300/45 bg-sky-100 text-sky-800 dark:border-sky-300/20 dark:bg-sky-500/10 dark:text-sky-200',
    sectionClass: 'border-sky-300/45 bg-[linear-gradient(180deg,rgba(125,211,252,0.12),rgba(125,211,252,0.03))] dark:border-sky-300/20 dark:bg-[linear-gradient(180deg,rgba(14,165,233,0.14),rgba(14,165,233,0.03))]',
    dividerClass: 'border-sky-300/35 dark:border-sky-300/15',
    glowClass: 'shadow-[0_0_0_1px_rgba(14,165,233,0.06)]',
    chartTintClass: 'bg-[radial-gradient(circle_at_top_left,rgba(56,189,248,0.16),transparent_46%),linear-gradient(180deg,rgba(255,255,255,0.03),rgba(255,255,255,0.01))]',
  },
}

const VARIANT_CONFIG = {
  A: {
    key: 'A',
    label: '组合A',
    shortLabel: 'A',
  },
  B: {
    key: 'B',
    label: '组合B',
    shortLabel: 'B',
  },
}

const METRIC_DEFINITIONS = [
  {
    key: 'latest_portfolio_return_pct',
    label: '累计收益',
    tooltip: '模拟组合从成立以来持有到当前的总收益率。',
    format: formatRankingPortfolioPercent,
    valueClass: getRankingPortfolioPerformanceClass,
    featured: true,
  },
  {
    key: 'current_month_return_pct',
    label: '本月收益率',
    tooltip: '自然月内按收盘价口径统计的组合收益率；若成立未满一个月，则与累计收益率一致。',
    format: formatRankingPortfolioPercent,
    valueClass: getRankingPortfolioPerformanceClass,
    featured: true,
  },
  {
    key: 'max_drawdown_pct',
    label: '最大回撤',
    tooltip: '成立以来从任一历史高点回落的最大跌幅，用于衡量最差持有体验。',
    format: formatRankingPortfolioPercent,
    valueClass: getNeutralMetricClass,
    featured: true,
  },
  {
    key: 'daily_win_rate_pct',
    label: '日胜率',
    tooltip: '成立以来盈利交易日占比，按正收益交易日数除以全部有收益记录的交易日数计算。',
    format: formatRankingPortfolioPercent,
    valueClass: getWinRateClass,
    featured: true,
  },
  {
    key: 'latest_daily_return_pct',
    label: '昨日收益率',
    tooltip: '上一交易日相对前一持仓日收盘价的组合收益率，含调仓成本影响。',
    format: formatRankingPortfolioPercent,
    valueClass: getRankingPortfolioPerformanceClass,
  },
  {
    key: 'volatility_pct',
    label: '波动率',
    tooltip: '成立以来日收益率的年化波动率，反映组合日度起伏大小。',
    format: formatRankingPortfolioPercent,
    valueClass: getNeutralMetricClass,
  },
  {
    key: 'inception_days',
    label: '成立天数',
    tooltip: '按起始收盘日到最新收盘日的自然日跨度统计，帮助用户判断累计收益的观察区间。',
    format: formatRankingPortfolioDayCount,
    valueClass: getNeutralMetricClass,
    subtext: (meta) => (meta?.inception_trade_date ? `起始日 ${meta.inception_trade_date}` : ''),
  },
]

function formatChartTick(time) {
  const formatted = formatRankingPortfolioDate(time)
  if (!/^\d{4}-\d{2}-\d{2}$/.test(formatted)) return formatted

  const [, month, day] = formatted.split('-')
  return `${Number(month)}/${Number(day)}`
}

function buildCurrentConstituentHint(meta) {
  const closeDateLabel = formatCloseDateLabel(meta?.current_constituent_source_date || meta?.source_trade_date, meta?.ranking_time)
  const effectiveDate = formatRankingPortfolioDate(meta?.current_constituent_effective_time || meta?.holdings_effective_time || meta?.ranking_time)
  if (!closeDateLabel || effectiveDate === '--') return ''
  return `${closeDateLabel}，${effectiveDate} 开盘生效`
}

function buildTooltipPosition(point, container) {
  const tooltipWidth = 214
  const tooltipHeight = 116
  const offset = 14
  const maxLeft = Math.max((container?.clientWidth || 0) - tooltipWidth - 8, 8)
  const maxTop = Math.max((container?.clientHeight || 0) - tooltipHeight - 8, 8)

  return {
    left: Math.min(Math.max(point.x + offset, 8), maxLeft),
    top: Math.min(Math.max(point.y - tooltipHeight - offset, 8), maxTop),
  }
}

function getNeutralMetricClass(value) {
  return value === null || value === undefined || Number.isNaN(Number(value)) ? 'text-foreground-dim' : 'text-foreground'
}

function getWinRateClass(value) {
  if (value === null || value === undefined || Number.isNaN(Number(value))) return 'text-foreground-dim'
  return Number(value) >= 50 ? 'text-negative' : 'text-positive'
}

function formatRankingPortfolioDayCount(value) {
  if (value === null || value === undefined || Number.isNaN(Number(value))) return '--'
  return `${Number(value)}天`
}

function buildPortfolioCardViewModel(item) {
  if (!item || typeof item !== 'object') return null

  const meta = item?.meta || null
  const exchangeKey = String(meta?.exchange || '').toUpperCase() || 'ASHARE'
  const variantKey = String(meta?.portfolio_variant || '').toUpperCase() || 'A'
  const market = MARKET_CONFIG[exchangeKey] || MARKET_CONFIG.ASHARE
  const variant = VARIANT_CONFIG[variantKey] || VARIANT_CONFIG.A
  const windowText = Number(meta?.selection_window || 0) > 0 ? `TOP${meta.selection_window}` : 'TOP4'
  const compactMarketHint = exchangeKey === 'ASHARE' ? '剔除科创板 · ' : ''
  const selectionSummary = variantKey === 'B' ? `${compactMarketHint}${windowText} 连续上榜优先` : `${compactMarketHint}TOP4`

  return {
    id: `${exchangeKey}:${variantKey}`,
    item,
    meta,
    market,
    variant,
    exchangeKey,
    variantKey,
    selectionSummary,
    currentConstituentHint: buildCurrentConstituentHint(meta),
    series: Array.isArray(item?.series) ? item.series : [],
    constituents: Array.isArray(item?.constituents) ? item.constituents : [],
    latestRebalance: item?.latest_rebalance && typeof item.latest_rebalance === 'object' ? item.latest_rebalance : null,
  }
}

function buildMarketSections(portfolios) {
  return Object.values(MARKET_CONFIG).map((market) => {
    const cards = Object.values(VARIANT_CONFIG).map((variant) => {
      const matched = portfolios.find((item) => {
        const exchange = String(item?.meta?.exchange || '').toUpperCase() || 'ASHARE'
        const variantKey = String(item?.meta?.portfolio_variant || '').toUpperCase() || 'A'
        return exchange === market.key && variantKey === variant.key
      })
      return matched ? buildPortfolioCardViewModel(matched) : null
    })

    return {
      market,
      cards,
      hasData: cards.some(Boolean),
    }
  })
}

function RankingPortfolioChart({ series = [], accent = 'amber' }) {
  const { resolvedTheme } = useTheme()
  const containerRef = useRef(null)
  const chartRef = useRef(null)
  const chartData = useMemo(() => buildRankingPortfolioChartSeriesData(series), [series])
  const pointMap = useMemo(
    () => Object.fromEntries(chartData.points.map((item) => [item.date, item])),
    [chartData.points]
  )
  const [tooltip, setTooltip] = useState(null)

  useEffect(() => {
    if (!containerRef.current || !chartData.points.length) {
      setTooltip(null)
      if (chartRef.current) {
        chartRef.current.remove()
        chartRef.current = null
      }
      return undefined
    }

    let cleanup = () => {}
    let cancelled = false

    const lineColor = accent === 'sky' ? '#38bdf8' : CHART_COLORS.portfolio
    const crosshairColor = accent === 'sky' ? 'rgba(56,189,248,0.28)' : 'rgba(245,158,11,0.28)'
    const crosshairLineColor = accent === 'sky' ? 'rgba(56,189,248,0.22)' : 'rgba(245,158,11,0.22)'
    const markerColorClass = accent === 'sky' ? 'bg-sky-400' : 'bg-amber-400'

    const render = async () => {
      const { createChart, ColorType, CrosshairMode, LineStyle } = await import('lightweight-charts')
      if (cancelled || !containerRef.current) return

      if (chartRef.current) {
        chartRef.current.remove()
        chartRef.current = null
      }

      const chart = createChart(containerRef.current, {
        width: containerRef.current.clientWidth || 720,
        height: 208,
        layout: {
          background: { type: ColorType.Solid, color: 'transparent' },
          textColor: resolvedTheme === 'light' ? CHART_COLORS.textLight : CHART_COLORS.textDark,
          fontSize: 11,
        },
        grid: {
          vertLines: { color: CHART_COLORS.grid },
          horzLines: { color: CHART_COLORS.grid },
        },
        rightPriceScale: {
          borderColor: CHART_COLORS.border,
          scaleMargins: { top: 0.14, bottom: 0.12 },
          entireTextOnly: true,
        },
        timeScale: {
          borderColor: CHART_COLORS.border,
          timeVisible: true,
          secondsVisible: false,
          rightOffset: 2,
          tickMarkFormatter: formatChartTick,
        },
        crosshair: {
          mode: CrosshairMode.Normal,
          vertLine: {
            color: crosshairColor,
            labelBackgroundColor: 'rgba(15,23,42,0.92)',
          },
          horzLine: {
            color: crosshairLineColor,
            labelBackgroundColor: 'rgba(15,23,42,0.92)',
          },
        },
        localization: {
          priceFormatter: (value) => formatRankingPortfolioPercent(value),
        },
        handleScroll: { mouseWheel: true, pressedMouseMove: true, horzTouchDrag: true, vertTouchDrag: false },
        handleScale: { axisPressedMouseMove: true, mouseWheel: true, pinch: true },
      })

      const baselineSeries = chart.addLineSeries({
        color: CHART_COLORS.baseline,
        lineWidth: 1,
        lineStyle: LineStyle.Dashed,
        crosshairMarkerVisible: false,
        lastValueVisible: false,
        priceLineVisible: false,
      })

      const portfolioSeries = chart.addLineSeries({
        color: lineColor,
        lineWidth: 3,
        lastValueVisible: false,
        priceLineVisible: false,
        title: '模拟组合累计收益',
      })

      baselineSeries.setData(chartData.baseline)
      portfolioSeries.setData(chartData.portfolio)
      chart.timeScale().fitContent()
      chartRef.current = chart

      const updateTooltip = (point, item) => {
        if (!point || !item || !containerRef.current) {
          setTooltip(null)
          return
        }
        const position = buildTooltipPosition(point, containerRef.current)
        setTooltip({
          left: position.left,
          top: position.top,
          point: item,
          markerColorClass,
        })
      }

      const handleCrosshairMove = (param) => {
        if (!param?.point || !param?.time || !containerRef.current) {
          setTooltip(null)
          return
        }

        const bounds = {
          width: containerRef.current.clientWidth || 0,
          height: containerRef.current.clientHeight || 0,
        }

        if (param.point.x < 0 || param.point.y < 0 || param.point.x > bounds.width || param.point.y > bounds.height) {
          setTooltip(null)
          return
        }

        const item = pointMap[formatRankingPortfolioDate(param.time)] || null
        updateTooltip(param.point, item)
      }

      const handleMouseLeave = () => setTooltip(null)
      const resize = () => {
        if (!containerRef.current || !chartRef.current) return
        chartRef.current.applyOptions({ width: containerRef.current.clientWidth || 720 })
        chartRef.current.timeScale().fitContent()
      }

      chart.subscribeCrosshairMove(handleCrosshairMove)
      containerRef.current.addEventListener('mouseleave', handleMouseLeave)
      window.addEventListener('resize', resize)

      let resizeObserver = null
      if (typeof ResizeObserver !== 'undefined') {
        resizeObserver = new ResizeObserver(() => resize())
        resizeObserver.observe(containerRef.current)
      }

      cleanup = () => {
        chart.unsubscribeCrosshairMove(handleCrosshairMove)
        containerRef.current?.removeEventListener('mouseleave', handleMouseLeave)
        window.removeEventListener('resize', resize)
        resizeObserver?.disconnect()
        if (chartRef.current) {
          chartRef.current.remove()
          chartRef.current = null
        }
      }
    }

    render()
    return () => {
      cancelled = true
      cleanup()
    }
  }, [accent, chartData.baseline, chartData.points, chartData.portfolio, pointMap, resolvedTheme])

  if (!chartData.points.length) {
    return (
      <div className="flex min-h-[228px] items-center justify-center rounded-2xl border border-dashed border-border/70 bg-[var(--color-bg-hover)] text-sm text-foreground-dim">
        暂无模拟组合曲线
      </div>
    )
  }

  return (
    <div className="overflow-hidden rounded-2xl border border-border/70 bg-[var(--color-bg-hover)]">
      <div className="flex flex-wrap items-center gap-x-4 gap-y-2 border-b border-border px-4 py-3 text-[11px] text-foreground-dim">
        <ChartLegendItem colorClass={accent === 'sky' ? 'bg-sky-400' : 'bg-amber-400'} label="模拟组合累计收益" />
        <span className="text-foreground-dim">悬停查看单日收益与回撤</span>
      </div>

      <div className="relative px-2 pb-2 pt-3">
        <div ref={containerRef} className="h-[208px] w-full" role="img" aria-label="模拟组合累计收益图表" />
        {tooltip ? (
          <div
            className="pointer-events-none absolute z-10 min-w-[194px] rounded-xl border border-border bg-card/94 px-3 py-2 shadow-2xl backdrop-blur"
            style={{ left: tooltip.left, top: tooltip.top }}
          >
            <div className="text-[11px] text-foreground/42">{tooltip.point.sourceTradeDate ? `${tooltip.point.sourceTradeDate} 收盘` : tooltip.point.date}</div>
            <TooltipMetric label="累计收益" value={tooltip.point.portfolioReturnPct} colorClass={tooltip.markerColorClass} highlight />
            <TooltipMetric label="单日收益" value={tooltip.point.dailyPortfolioReturnPct} colorClass="bg-[var(--color-border-strong)]" />
            <TooltipMetric label="回撤" value={tooltip.point.drawdownPct} colorClass="bg-slate-300" />
          </div>
        ) : null}
      </div>
    </div>
  )
}

export default function RankingPortfolioPanel({ data = null, loading = false }) {
  const panelRef = useRef(null)
  const portfolios = Array.isArray(data?.items) ? data.items : []
  const marketSections = useMemo(() => buildMarketSections(portfolios), [portfolios])
  const hasAnyPortfolio = marketSections.some((section) => section.hasData)

  if (loading && portfolios.length === 0) {
    return (
      <section className="rounded-2xl border border-border bg-card p-5">
        <div className="flex h-56 items-center justify-center text-sm text-foreground-dim">
          <span className="animate-pulse">加载模拟组合收益...</span>
        </div>
      </section>
    )
  }

  return (
    <section ref={panelRef} className="rounded-2xl border border-border bg-card p-4 sm:p-5">
      <div className="flex flex-col gap-3 border-b border-border/80 pb-4">
        <div className="flex flex-wrap items-center gap-2">
          <h3 className="text-base font-semibold text-foreground">卧龙AI精选模拟组合</h3>
        </div>
      </div>

      {!hasAnyPortfolio ? (
        <div className="mt-4 rounded-xl border border-dashed border-border px-4 py-8 text-center text-sm text-foreground-dim">暂无模拟组合数据</div>
      ) : (
        <div className="mt-5 space-y-5">
          {marketSections.filter((section) => section.hasData).map((section) => (
            <MarketPortfolioSection key={section.market.key} section={section} />
          ))}
        </div>
      )}
    </section>
  )
}

function MarketPortfolioSection({ section }) {
  const { market, cards } = section

  return (
    <section className={`rounded-[28px] border p-4 sm:p-5 ${market.sectionClass} ${market.glowClass}`}>
      <div className={`flex flex-col gap-3 border-b pb-4 ${market.dividerClass}`}>
        <div className="flex flex-wrap items-center gap-2">
          <span className={`rounded-full border px-2.5 py-1 text-[11px] font-semibold ${market.badgeClass}`}>{market.label}</span>
          <h4 className={`text-lg font-semibold tracking-tight ${market.accentClass}`}>{market.sectionTitle}</h4>
        </div>
      </div>

      <div className="mt-4 grid gap-4 xl:grid-cols-2">
        {cards.map((card, index) => (
          card ? <PortfolioOverviewCard key={card.id} card={card} /> : <PortfolioPlaceholderCard key={`${market.key}:${index}`} market={market} variant={Object.values(VARIANT_CONFIG)[index]} />
        ))}
      </div>
    </section>
  )
}

function PortfolioOverviewCard({ card }) {
  const { meta, market, variant, selectionSummary, currentConstituentHint, constituents, latestRebalance, series } = card
  const featuredMetrics = METRIC_DEFINITIONS.filter((metric) => metric.featured)
  const secondaryMetrics = METRIC_DEFINITIONS.filter((metric) => !metric.featured)
  const visibleConstituents = constituents.slice(0, 4)
  const chartAccent = market.key === 'HKEX' ? 'sky' : 'amber'

  return (
    <article className="flex h-full flex-col rounded-[26px] border border-border/80 bg-card/92 p-4 shadow-sm backdrop-blur sm:p-4.5">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="min-w-0 flex-1">
          <div className="flex flex-wrap items-center gap-2">
            <span className={`rounded-full border px-2 py-0.5 text-[11px] font-medium ${market.badgeClass}`}>{market.label}</span>
            <span className="rounded-full border border-border bg-[var(--color-bg-hover)] px-2 py-0.5 text-[11px] font-medium text-foreground-muted">{variant.label}</span>
          </div>
          <h5 className="mt-2 text-base font-semibold text-foreground">{meta?.name || `模拟${variant.label}`}</h5>
          <div className="mt-2 flex flex-wrap items-center gap-2 text-[11px] text-foreground-dim">
            <span className="rounded-full border border-border bg-[var(--color-bg-hover)] px-2 py-1">{selectionSummary}</span>
            {meta?.inception_trade_date ? <span className="rounded-full border border-border bg-[var(--color-bg-hover)] px-2 py-1">起始日 {meta.inception_trade_date}</span> : null}
          </div>
        </div>
        <div className="rounded-2xl border border-border bg-[var(--color-bg-hover)] px-3 py-2 text-right">
          <div className="text-[11px] text-foreground-dim">累计收益</div>
          <div className={`mt-1 text-lg font-semibold tabular-nums ${getRankingPortfolioPerformanceClass(meta?.latest_portfolio_return_pct)}`}>
            {formatRankingPortfolioPercent(meta?.latest_portfolio_return_pct)}
          </div>
        </div>
      </div>

      <div className="mt-4 grid grid-cols-2 gap-2">
        {featuredMetrics.map((metric) => (
          <MetricCard
            key={metric.key}
            label={metric.label}
            tooltip={metric.tooltip}
            value={metric.format(meta?.[metric.key])}
            valueClass={metric.valueClass(meta?.[metric.key])}
            subtext={metric.subtext ? metric.subtext(meta) : ''}
          />
        ))}
      </div>

      <div className="mt-3 grid gap-2 sm:grid-cols-3">
        {secondaryMetrics.map((metric) => (
          <CompactMetricItem
            key={metric.key}
            label={metric.label}
            value={metric.format(meta?.[metric.key])}
            valueClass={metric.valueClass(meta?.[metric.key])}
          />
        ))}
      </div>

      <div className={`mt-4 rounded-2xl border border-border/70 p-3 ${market.chartTintClass}`}>
        <div className="mb-2 flex items-center justify-between gap-2">
          <div>
            <div className="text-sm font-medium text-foreground">收益曲线</div>
            {meta?.source_trade_date ? <div className="mt-1 text-[11px] leading-5 text-foreground/56 dark:text-foreground/42">截至 {formatCloseDateLabel(meta?.source_trade_date, meta?.ranking_time)} · 按开盘价模拟买入、收盘价模拟调仓</div> : null}
          </div>
        </div>
        <RankingPortfolioChart series={series} accent={chartAccent} />
      </div>

      <div className="mt-4 grid gap-3 lg:grid-cols-[minmax(0,1fr)_minmax(0,0.92fr)]">
        <div className="rounded-2xl border border-border/70 bg-[var(--color-bg-hover)] p-3.5">
          <div className="text-sm font-medium text-foreground">当前成分股</div>
          {currentConstituentHint ? <div className="mt-1 text-[11px] leading-5 text-foreground/56 dark:text-foreground/42">{currentConstituentHint}</div> : null}
          {meta?.is_same_batch_as_performance === false && meta?.batch_mismatch_reason ? <div className="mt-2 rounded-xl border border-sky-400/35 bg-sky-100 px-3 py-2 text-xs text-sky-700 dark:border-sky-300/20 dark:bg-sky-500/10 dark:text-sky-200">{meta.batch_mismatch_reason}</div> : null}
          {meta?.has_shortfall ? (
            <div className="mt-2 rounded-xl border border-amber-300/20 bg-amber-500/10 px-3 py-2 text-xs text-amber-200">
              {meta?.warning_text || '当日有效成分股不足 4 只'}
            </div>
          ) : null}
          <div className="mt-3 space-y-2">
            {visibleConstituents.length ? visibleConstituents.map((item) => (
              <RankingPortfolioConstituentRow key={`${item.exchange}-${item.code}`} item={item} showSourceRank={card.variantKey === 'B'} compact />
            )) : (
              <div className="rounded-xl border border-dashed border-border/70 px-4 py-8 text-center text-sm text-foreground-dim">
                暂无成分股数据
              </div>
            )}
          </div>
        </div>

        <div className="rounded-2xl border border-border/70 bg-[var(--color-bg-hover)] p-3.5">
          <div className="flex items-center gap-2">
            <div className="text-sm font-medium text-foreground">最近一次调仓</div>
            {latestRebalance ? (
              <span className="rounded-full bg-[var(--color-bg-secondary)] px-1.5 py-0.5 text-[10px] text-foreground-dim">
                {Number(latestRebalance.change_count || (Array.isArray(latestRebalance.items) ? latestRebalance.items.length : 0) || 0)}项
              </span>
            ) : null}
          </div>
          <LatestRebalanceDisclosure rebalance={latestRebalance} compact />
        </div>
      </div>
    </article>
  )
}

function PortfolioPlaceholderCard({ market, variant }) {
  return (
    <article className="flex min-h-[520px] flex-col items-center justify-center rounded-[26px] border border-dashed border-border/70 bg-card/80 px-6 py-8 text-center">
      <div className="flex flex-wrap items-center justify-center gap-2">
        <span className={`rounded-full border px-2 py-0.5 text-[11px] font-medium ${market.badgeClass}`}>{market.label}</span>
        <span className="rounded-full border border-border bg-[var(--color-bg-hover)] px-2 py-0.5 text-[11px] font-medium text-foreground-muted">{variant.label}</span>
      </div>
      <div className="mt-4 text-sm font-medium text-foreground">暂无该组合数据</div>
      <div className="mt-2 max-w-xs text-xs leading-5 text-foreground-dim">保留卡片位置，确保 A股 / 港股与 A / B 组合的布局稳定，避免数据缺失时页面跳动。</div>
    </article>
  )
}

function ChartLegendItem({ colorClass, label }) {
  return (
    <div className="inline-flex items-center gap-2">
      <span className={`h-2.5 w-2.5 rounded-full ${colorClass}`} aria-hidden="true" />
      <span>{label}</span>
    </div>
  )
}

function TooltipMetric({ label, value, colorClass, highlight = false }) {
  return (
    <div className="mt-1.5 flex items-center justify-between gap-3 text-[11px] tabular-nums">
      <div className="inline-flex items-center gap-2 text-foreground/62">
        <span className={`h-2 w-2 rounded-full ${colorClass}`} aria-hidden="true" />
        <span>{label}</span>
      </div>
      <div className={highlight ? getRankingPortfolioPerformanceClass(value) : 'text-foreground'}>{formatRankingPortfolioPercent(value)}</div>
    </div>
  )
}

function MetricCard({ label, tooltip, value, valueClass, subtext = '' }) {
  return (
    <div className="rounded-xl border border-border bg-[var(--color-bg-hover)] px-2.5 py-2.5">
      <div className="text-[11px] text-foreground-dim">
        <LabelWithInfo label={label} tooltip={tooltip} labelClassName="text-[11px] text-foreground-dim" tipPlacement="top-right" tipWidthClassName="w-52 sm:w-60" />
      </div>
      <div className={`mt-1.5 text-base font-semibold tabular-nums ${valueClass}`}>{value}</div>
      {subtext ? <div className="mt-1 text-[11px] text-foreground-dim">{subtext}</div> : null}
    </div>
  )
}

function CompactMetricItem({ label, value, valueClass }) {
  return (
    <div className="rounded-xl border border-border/70 bg-card/70 px-3 py-2.5">
      <div className="text-[11px] text-foreground-dim">{label}</div>
      <div className={`mt-1 text-sm font-semibold tabular-nums ${valueClass}`}>{value}</div>
    </div>
  )
}

function RankingPortfolioConstituentRow({ item, showSourceRank = false, compact = false }) {
  const detailHref = buildRankingPortfolioDetailHref(item?.code, item?.exchange)
  const codeLabel = formatRankingPortfolioCode(item?.code, item?.exchange)
  const sourceMeta = showSourceRank
    ? `榜单第${item?.source_rank || '--'}名 · 连续${item?.consecutive_days || '--'}日`
    : ''
  const isEntryPending = Boolean(item?.entry_price_pending)
  const returnValue = isEntryPending ? '待开盘' : formatRankingPortfolioPercent(item?.latest_return_pct)
  const returnClass = isEntryPending ? 'text-foreground-dim' : getRankingPortfolioPerformanceClass(item?.latest_return_pct)
  const latestDisplayPrice = item?.latest_price || item?.latest_close_price
  const priceSummaryParts = []
  if (isEntryPending) {
    priceSummaryParts.push('买入价待开盘')
  } else if (item?.entry_price) {
    priceSummaryParts.push(`买入价 ${formatRankingPortfolioReferencePrice(item?.entry_price, item?.exchange)}`)
  }
  if (latestDisplayPrice) {
    priceSummaryParts.push(`最新 ${formatRankingPortfolioReferencePrice(latestDisplayPrice, item?.exchange)}`)
  }
  const priceSummary = priceSummaryParts.join(' · ')
  const rowClass = compact ? 'px-3 py-2.5' : 'px-3 py-2'
  const content = (
    <>
      <div className="min-w-0 flex-1">
        <div className="truncate text-sm font-medium text-foreground transition group-hover:text-primary dark:group-hover:text-amber-100">{item?.name || '--'}</div>
        <div className="mt-0.5 text-[11px] text-foreground-dim">#{item?.rank} · {codeLabel}</div>
        {showSourceRank ? <div className="mt-0.5 text-[11px] text-foreground-dim">{sourceMeta}</div> : null}
        {priceSummary ? <div className="mt-1 text-[11px] text-foreground-dim">{priceSummary}</div> : null}
      </div>
      <div className="pl-3 text-right">
        <div className={`text-sm font-semibold tabular-nums ${returnClass}`}>{returnValue}</div>
        <div className="mt-0.5 text-[11px] text-foreground-dim">较买入价</div>
        <div className="mt-1 text-[11px] text-foreground-dim">仓位 {formatRankingPortfolioPercent((item?.weight || 0) * 100, 0)}</div>
      </div>
    </>
  )

  if (!detailHref) {
    return <div className={`flex items-center justify-between rounded-xl border border-border bg-card/80 ${rowClass}`}>{content}</div>
  }

  return (
    <a
      href={detailHref}
      target="_blank"
      rel="noreferrer"
      title={`查看 ${item?.name || formatRankingPortfolioCode(item?.code, item?.exchange)} 详情`}
      className={`group flex items-center justify-between rounded-xl border border-border bg-card/80 ${rowClass} transition hover:border-primary/30 dark:hover:border-amber-300/30 hover:bg-primary/[0.04] dark:hover:bg-amber-500/[0.06] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary/60 dark:focus-visible:ring-amber-300/60`}
    >
      {content}
    </a>
  )
}

function LatestRebalanceDisclosure({ rebalance, compact = false }) {
  if (!rebalance) {
    return (
      <div className="mt-3 rounded-xl border border-dashed border-border/70 px-3 py-6 text-center text-sm text-foreground-dim">
        暂无调仓记录
      </div>
    )
  }

  const items = Array.isArray(rebalance?.items) ? rebalance.items : []
  const effectiveTime = formatRankingPortfolioDateTime(rebalance?.effective_time)
  const tradeCostRate = Number(rebalance?.trade_cost_rate || 0)

  return (
    <div className="mt-2 rounded-xl border border-border bg-card/80 px-3 py-3">
      <div className="text-[11px] leading-5 text-foreground/42">
        生效时间：{effectiveTime}
        {tradeCostRate > 0 ? ` · 含 ${formatRankingPortfolioPercent(tradeCostRate * 100, 2)} 交易成本` : ''}
      </div>

      {items.length ? (
        <div className="mt-3 space-y-2">
          {(compact ? items.slice(0, 4) : items).map((item) => (
            <LatestRebalanceRow key={`${item.exchange}-${item.code}-${item.action}`} item={item} />
          ))}
          {compact && items.length > 4 ? <div className="text-[11px] text-foreground-dim">已折叠其余 {items.length - 4} 项</div> : null}
        </div>
      ) : (
        <div className="mt-3 rounded-xl border border-dashed border-border/70 px-3 py-4 text-center text-sm text-foreground-dim">
          本次未发生调仓
        </div>
      )}
    </div>
  )
}

function LatestRebalanceRow({ item }) {
  const isSell = String(item?.action || '').toLowerCase() === 'sell'
  const badgeClass = isSell
    ? 'border-emerald-400/35 bg-emerald-100 text-emerald-700 dark:border-positive/20 dark:bg-positive/10 dark:text-positive'
    : 'border-amber-400/40 bg-amber-100 text-amber-700 dark:border-amber-300/20 dark:bg-amber-500/10 dark:text-amber-200'

  const hasSoldReturn = isSell && item?.sold_return_pct !== null && item?.sold_return_pct !== undefined
  const soldReturnValue = hasSoldReturn ? item.sold_return_pct : null
  const soldReturnClass = soldReturnValue !== null
    ? getRankingPortfolioPerformanceClass(soldReturnValue)
    : 'text-foreground-dim'
  const soldReturnLabel = soldReturnValue !== null
    ? formatRankingPortfolioPercent(soldReturnValue)
    : '--'

  return (
    <div className="rounded-xl border border-border bg-[var(--color-bg-hover)] px-3 py-2.5">
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0">
          <div className="flex flex-wrap items-center gap-2">
            <span className={`rounded-full border px-2 py-0.5 text-[11px] ${badgeClass}`}>{getRankingPortfolioRebalanceActionLabel(item?.action)}</span>
            <span className="truncate text-sm font-medium text-foreground">{item?.name || '--'}</span>
          </div>
          <div className="mt-1 text-[11px] text-foreground-dim">{formatRankingPortfolioCode(item?.code, item?.exchange)}</div>
        </div>

        <div className="text-right text-[11px] text-foreground/42">
          <div>仓位 {formatRankingPortfolioWeightChange(item?.from_weight, item?.to_weight, 0)}</div>
          <div className="mt-1">参考成本价 {formatRankingPortfolioReferencePrice(item?.reference_cost_price, item?.exchange)}</div>
          {isSell ? (
            <div className={`mt-1 ${soldReturnClass}`} title="从最近一次买入开盘价起算，含单边卖出成本">
              回报率 {soldReturnValue !== null ? `${soldReturnLabel}（参考）` : '--'}
            </div>
          ) : null}
        </div>
      </div>
    </div>
  )
}
