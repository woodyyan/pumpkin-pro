import { useEffect, useMemo, useRef, useState } from 'react'

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
  benchmark: '#cbd5e1',
  baseline: 'rgba(148,163,184,0.35)',
  grid: 'rgba(148,163,184,0.10)',
  border: 'rgba(148,163,184,0.20)',
  text: 'rgba(255,255,255,0.72)',
}

function formatChartTick(time) {
  const formatted = formatRankingPortfolioDate(time)
  if (!/^\d{4}-\d{2}-\d{2}$/.test(formatted)) return formatted

  const [, month, day] = formatted.split('-')
  return `${Number(month)}/${Number(day)}`
}

function buildCurrentConstituentHint(meta) {
  const closeDateLabel = formatCloseDateLabel(meta?.current_constituent_source_date || meta?.source_trade_date, meta?.ranking_time)
  const effectiveDate = formatChartTick(meta?.current_constituent_effective_time || meta?.holdings_effective_time)

  if (!closeDateLabel || !effectiveDate || effectiveDate === '--') return ''
  return `${closeDateLabel}，${effectiveDate} 开盘生效`
}

function buildTooltipPosition(point, container) {
  const tooltipWidth = 196
  const tooltipHeight = 106
  const offset = 14
  const maxLeft = Math.max((container?.clientWidth || 0) - tooltipWidth - 8, 8)
  const maxTop = Math.max((container?.clientHeight || 0) - tooltipHeight - 8, 8)

  return {
    left: Math.min(Math.max(point.x + offset, 8), maxLeft),
    top: Math.min(Math.max(point.y - tooltipHeight - offset, 8), maxTop),
  }
}

function RankingPortfolioChart({ series = [], benchmarkLabel = '上证指数' }) {
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

    const render = async () => {
      const { createChart, ColorType, CrosshairMode, LineStyle } = await import('lightweight-charts')
      if (cancelled || !containerRef.current) return

      if (chartRef.current) {
        chartRef.current.remove()
        chartRef.current = null
      }

      const chart = createChart(containerRef.current, {
        width: containerRef.current.clientWidth || 720,
        height: 280,
        layout: {
          background: { type: ColorType.Solid, color: 'transparent' },
          textColor: CHART_COLORS.text,
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
            color: 'rgba(245,158,11,0.28)',
            labelBackgroundColor: 'rgba(15,23,42,0.92)',
          },
          horzLine: {
            color: 'rgba(245,158,11,0.22)',
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

      const benchmarkSeries = chart.addLineSeries({
        color: CHART_COLORS.benchmark,
        lineWidth: 2,
        lastValueVisible: false,
        priceLineVisible: false,
        title: `${benchmarkLabel}累计收益`,
      })

      const portfolioSeries = chart.addLineSeries({
        color: CHART_COLORS.portfolio,
        lineWidth: 3,
        lastValueVisible: false,
        priceLineVisible: false,
        title: '模拟组合累计收益',
      })

      baselineSeries.setData(chartData.baseline)
      benchmarkSeries.setData(chartData.benchmark)
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
  }, [benchmarkLabel, chartData.baseline, chartData.benchmark, chartData.points.length, chartData.portfolio, pointMap])

  if (!chartData.points.length) {
    return (
      <div className="flex min-h-[300px] items-center justify-center rounded-2xl border border-dashed border-border/70 bg-[var(--color-bg-hover)] text-sm text-foreground-dim xl:h-full">
        暂无模拟组合曲线
      </div>
    )
  }

  return (
    <div className="overflow-hidden rounded-2xl border border-border/70 bg-[radial-gradient(circle_at_top_left,rgba(245,158,11,0.14),transparent_45%),linear-gradient(180deg,rgba(255,255,255,0.03),rgba(255,255,255,0.01))]">
      <div className="flex flex-wrap items-center gap-x-4 gap-y-2 border-b border-border px-4 py-3 text-[11px] text-foreground-dim">
        <ChartLegendItem colorClass="bg-amber-400" label="模拟组合累计收益" />
        <ChartLegendItem colorClass="bg-slate-300" label={`${benchmarkLabel}累计收益`} />
        <span className="text-foreground-dim">横轴时间 · 纵轴累计收益</span>
      </div>

      <div className="relative px-2 pb-2 pt-3">
        <div ref={containerRef} className="h-[280px] w-full" role="img" aria-label="模拟组合与基准累计收益图表" />
        {tooltip ? (
          <div
            className="pointer-events-none absolute z-10 min-w-[180px] rounded-xl border border-border bg-card/94 px-3 py-2 shadow-2xl backdrop-blur"
            style={{ left: tooltip.left, top: tooltip.top }}
          >
            <div className="text-[11px] text-foreground/42">{tooltip.point.date}</div>
            <TooltipMetric label="模拟组合" value={tooltip.point.portfolioReturnPct} colorClass="bg-amber-400" />
            <TooltipMetric label={benchmarkLabel} value={tooltip.point.benchmarkReturnPct} colorClass="bg-slate-300" />
            <TooltipMetric label="超额收益" value={tooltip.point.excessReturnPct} colorClass="bg-sky-400" highlight />
          </div>
        ) : null}
      </div>
    </div>
  )
}

export default function RankingPortfolioPanel({ data = null, loading = false }) {
  const panelRef = useRef(null)
  const [selectedExchange, setSelectedExchange] = useState('ASHARE')
  const [selectedVariant, setSelectedVariant] = useState('A')
  const portfolios = Array.isArray(data?.items) ? data.items : []
  const exchangeTabs = [
    { key: 'ASHARE', label: 'A股' },
    { key: 'HKEX', label: '港股' },
  ]
  const variantTabs = [
    { key: 'A', label: '模拟组合A' },
    { key: 'B', label: '模拟组合B' },
  ]

  const portfolioMap = useMemo(() => {
    const entries = {}
    for (const item of portfolios) {
      const exchange = String(item?.meta?.exchange || '').toUpperCase() || 'ASHARE'
      const variant = String(item?.meta?.portfolio_variant || '').toUpperCase() || 'A'
      entries[`${exchange}:${variant}`] = item
    }
    return entries
  }, [portfolios])

  const availableExchanges = useMemo(
    () => exchangeTabs.filter((tab) => portfolioMap[`${tab.key}:A`] || portfolioMap[`${tab.key}:B`]),
    [portfolioMap]
  )

  const availableVariants = useMemo(
    () => variantTabs.filter((tab) => portfolioMap[`${selectedExchange}:${tab.key}`]),
    [portfolioMap, selectedExchange]
  )

  useEffect(() => {
    if (!availableExchanges.length) return
    if (!availableExchanges.some((tab) => tab.key === selectedExchange)) {
      setSelectedExchange(availableExchanges[0].key)
    }
  }, [availableExchanges, selectedExchange])

  useEffect(() => {
    if (!availableVariants.length) return
    if (!availableVariants.some((tab) => tab.key === selectedVariant)) {
      setSelectedVariant(availableVariants[0].key)
    }
  }, [availableVariants, selectedVariant])

  const selectedPortfolio = portfolioMap[`${selectedExchange}:${selectedVariant}`] || portfolios[0] || null
  const meta = selectedPortfolio?.meta || null
  const series = Array.isArray(selectedPortfolio?.series) ? selectedPortfolio.series : []
  const constituents = Array.isArray(selectedPortfolio?.constituents) ? selectedPortfolio.constituents : []
  const latestRebalance = selectedPortfolio?.latest_rebalance && typeof selectedPortfolio.latest_rebalance === 'object'
    ? selectedPortfolio.latest_rebalance
    : null
  const benchmarkLabel = meta?.benchmark_name || meta?.benchmark_code || '上证指数'
  const windowText = Number(meta?.selection_window || 0) > 0 ? `TOP${meta.selection_window}` : 'TOP4'
  const compactMarketHint = selectedExchange === 'ASHARE' ? '剔除科创板 · ' : ''
  const currentConstituentHint = buildCurrentConstituentHint(meta)
  const selectionSummary = meta?.portfolio_variant === 'B'
    ? `${compactMarketHint}${windowText} 连续上榜优先`
    : `${compactMarketHint}TOP4`

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
      <div className="flex flex-col gap-3 lg:flex-row lg:items-start lg:justify-between">
        <div className="min-w-0 flex-1">
          <div className="flex flex-wrap items-center gap-2">
            <h3 className="text-base font-semibold text-foreground">卧龙AI精选模拟组合</h3>
            <span className="rounded-full border border-amber-300/40 dark:border-amber-300/20 bg-amber-100 dark:bg-amber-500/10 px-2 py-0.5 text-[11px] text-amber-800 dark:text-amber-200 font-medium">A / B 双组合</span>
          </div>
          <p className="mt-1 text-xs leading-5 text-foreground-dim">跟踪卧龙AI精选 A、B 两套组合表现，快速看哪套规则更稳、哪套更能跑赢基准。</p>
        </div>

        <div className="flex flex-wrap items-center gap-2 lg:justify-end">
          <div className="flex items-center gap-1 rounded-lg bg-[var(--color-bg-hover)] p-0.5">
            {exchangeTabs.map((tab) => {
              const disabled = !portfolioMap[`${tab.key}:A`] && !portfolioMap[`${tab.key}:B`]
              return (
                <button
                  key={tab.key}
                  type="button"
                  disabled={disabled}
                  onClick={() => setSelectedExchange(tab.key)}
                  className={`rounded-md px-3 py-1 text-xs font-medium transition ${
                    selectedExchange === tab.key
                      ? 'bg-primary text-black'
                      : 'text-foreground-dim hover:bg-[var(--color-bg-hover)] hover:text-foreground-muted'
                  } ${disabled ? 'cursor-not-allowed opacity-35 hover:bg-transparent hover:text-foreground-dim' : ''}`}
                >
                  {tab.label}
                </button>
              )
            })}
          </div>

          <div className="flex items-center gap-1 rounded-lg bg-[var(--color-bg-hover)] p-0.5">
            {variantTabs.map((tab) => {
              const disabled = !portfolioMap[`${selectedExchange}:${tab.key}`]
              return (
                <button
                  key={tab.key}
                  type="button"
                  disabled={disabled}
                  onClick={() => setSelectedVariant(tab.key)}
                  className={`rounded-md px-3 py-1 text-xs font-medium transition ${
                    selectedVariant === tab.key
                      ? 'bg-white text-slate-950'
                      : 'text-foreground-dim hover:bg-[var(--color-bg-hover)] hover:text-foreground-muted'
                  } ${disabled ? 'cursor-not-allowed opacity-35 hover:bg-transparent hover:text-foreground-dim' : ''}`}
                >
                  {tab.label}
                </button>
              )
            })}
          </div>
        </div>
      </div>

      {!selectedPortfolio ? (
        <div className="mt-4 rounded-xl border border-dashed border-border px-4 py-8 text-center text-sm text-foreground-dim">暂无模拟组合数据</div>
      ) : (
        <>
          <div className="mt-3 rounded-2xl border border-border bg-[var(--color-bg-hover)] px-3 py-3">
            <div className="flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
              <div className="flex flex-wrap items-center gap-2 text-[11px] text-foreground-dim">
                <span className="rounded-full border border-border bg-[var(--color-bg-hover)] px-2 py-1 text-foreground-muted">{selectedExchange === 'HKEX' ? '港股' : 'A股'} {meta?.name || '模拟组合'}</span>
                <span className="rounded-full border border-border bg-[var(--color-bg-hover)] px-2 py-1">{selectionSummary}</span>
              </div>

              <div className="grid grid-cols-3 gap-2 lg:min-w-[300px]">
                <MetricCard
                  label="累计收益"
                  tooltip="模拟组合从起始日持有到当前的总收益率，反映这套选股规则本身的累计表现。"
                  value={formatRankingPortfolioPercent(meta?.latest_portfolio_return_pct)}
                  valueClass={getRankingPortfolioPerformanceClass(meta?.latest_portfolio_return_pct)}
                />
                <MetricCard
                  label="基准收益"
                  tooltip={`同一时间段内，基准 ${benchmarkLabel} 的累计收益率，用来对比市场整体表现。`}
                  value={formatRankingPortfolioPercent(meta?.latest_benchmark_return_pct)}
                  valueClass={getRankingPortfolioPerformanceClass(meta?.latest_benchmark_return_pct)}
                />
                <MetricCard
                  label="超额收益"
                  tooltip={`模拟组合累计收益减去 ${benchmarkLabel} 累计收益后的差值。正值表示跑赢基准，负值表示跑输基准。`}
                  value={formatRankingPortfolioPercent(meta?.latest_excess_return_pct)}
                  valueClass={getRankingPortfolioPerformanceClass(meta?.latest_excess_return_pct)}
                />
              </div>
            </div>
          </div>

          <div className="mt-3 grid gap-3 xl:grid-cols-[minmax(0,1.52fr)_minmax(270px,0.82fr)] xl:items-stretch">
            <div className="xl:h-full">
              <RankingPortfolioChart series={series} benchmarkLabel={benchmarkLabel} />
            </div>

            <div className="rounded-2xl border border-border/70 bg-[var(--color-bg-hover)] p-3.5 xl:h-full">
              <div className="text-sm font-medium text-foreground">当前成分股</div>
              {currentConstituentHint ? <div className="mt-1 text-[11px] leading-5 text-foreground/42">{currentConstituentHint}</div> : null}

              {meta?.has_shortfall ? (
                <div className="mt-3 rounded-xl border border-amber-300/20 bg-amber-500/10 px-3 py-2 text-xs text-amber-200">
                  {meta?.warning_text || '当日有效成分股不足 4 只'}
                </div>
              ) : null}

              <div className="mt-3 space-y-2">
                {constituents.length ? constituents.map((item) => (
                  <RankingPortfolioConstituentRow key={`${item.exchange}-${item.code}`} item={item} showSourceRank={meta?.portfolio_variant === 'B'} />
                )) : (
                  <div className="rounded-xl border border-dashed border-border/70 px-4 py-8 text-center text-sm text-foreground-dim">
                    暂无成分股数据
                  </div>
                )}
              </div>

              <LatestRebalanceDisclosure rebalance={latestRebalance} />
            </div>
          </div>
        </>
      )}
    </section>
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

function MetricCard({ label, tooltip, value, valueClass }) {
  return (
    <div className="rounded-xl border border-border bg-[var(--color-bg-hover)] px-2.5 py-2.5">
      <div className="text-[11px] text-foreground-dim">
        <LabelWithInfo label={label} tooltip={tooltip} labelClassName="text-[11px] text-foreground-dim" tipPlacement="top-right" tipWidthClassName="w-52 sm:w-60" />
      </div>
      <div className={`mt-1.5 text-base font-semibold tabular-nums ${valueClass}`}>{value}</div>
    </div>
  )
}

function RankingPortfolioConstituentRow({ item, showSourceRank = false }) {
  const detailHref = buildRankingPortfolioDetailHref(item?.code, item?.exchange)
  const codeLabel = formatRankingPortfolioCode(item?.code, item?.exchange)
  const sourceMeta = showSourceRank
    ? `榜单第${item?.source_rank || '--'}名 · 连续${item?.consecutive_days || '--'}日`
    : ''
  const content = (
    <>
      <div className="min-w-0">
        <div className="truncate text-sm font-medium text-foreground transition group-hover:text-amber-100">{item?.name || '--'}</div>
        <div className="mt-0.5 text-[11px] text-foreground-dim">#{item?.rank} · {codeLabel}</div>
        {showSourceRank ? <div className="mt-0.5 text-[11px] text-foreground-dim">{sourceMeta}</div> : null}
      </div>
      <div className="text-right">
        <div className="text-sm font-semibold text-foreground">{formatRankingPortfolioPercent((item?.weight || 0) * 100, 0)}</div>
        <div className="mt-0.5 text-[11px] text-foreground-dim">仓位</div>
      </div>
    </>
  )

  if (!detailHref) {
    return <div className="flex items-center justify-between rounded-xl border border-border bg-[var(--color-bg-hover)] px-3 py-2">{content}</div>
  }

  return (
    <a
      href={detailHref}
      target="_blank"
      rel="noreferrer"
      title={`查看 ${item?.name || formatRankingPortfolioCode(item?.code, item?.exchange)} 详情`}
      className="group flex items-center justify-between rounded-xl border border-border bg-[var(--color-bg-hover)] px-3 py-2 transition hover:border-amber-300/30 hover:bg-amber-500/[0.06] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-amber-300/60"
    >
      {content}
    </a>
  )
}

function LatestRebalanceDisclosure({ rebalance }) {
  if (!rebalance) return null

  const items = Array.isArray(rebalance?.items) ? rebalance.items : []
  const changeCount = Number(rebalance?.change_count || items.length || 0)
  const effectiveTime = formatRankingPortfolioDateTime(rebalance?.effective_time)
  const tradeCostRate = Number(rebalance?.trade_cost_rate || 0)

  return (
    <details className="mt-3">
      <summary className="inline-flex cursor-pointer list-none items-center gap-2 rounded-full border border-border bg-[var(--color-bg-hover)] px-2.5 py-1 text-[11px] text-foreground-disabled2 transition hover:border-white/18 hover:text-foreground/72 marker:hidden">
        <span>最近一次调仓</span>
        <span className="rounded-full bg-[var(--color-bg-secondary)] px-1.5 py-0.5 text-[10px] text-foreground-dim">{changeCount}项</span>
      </summary>

      <div className="mt-2 rounded-xl border border-border bg-[var(--color-bg-hover)] px-3 py-3">
        <div className="text-[11px] leading-5 text-foreground/42">
          生效时间：{effectiveTime}
          {tradeCostRate > 0 ? ` · 含 ${formatRankingPortfolioPercent(tradeCostRate * 100, 2)} 交易成本` : ''}
        </div>

        {items.length ? (
          <div className="mt-3 space-y-2">
            {items.map((item) => (
              <LatestRebalanceRow key={`${item.exchange}-${item.code}-${item.action}`} item={item} />
            ))}
          </div>
        ) : (
          <div className="mt-3 rounded-xl border border-dashed border-border/70 px-3 py-4 text-center text-sm text-foreground-dim">
            本次未发生调仓
          </div>
        )}
      </div>
    </details>
  )
}

function LatestRebalanceRow({ item }) {
  const isSell = String(item?.action || '').toLowerCase() === 'sell'
  const badgeClass = isSell
    ? 'border-positive/20 bg-positive/10 text-positive'
    : 'border-amber-300/20 bg-amber-500/10 text-amber-200'

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
        </div>
      </div>
    </div>
  )
}
