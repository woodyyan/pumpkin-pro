import { useEffect, useMemo, useRef, useState } from 'react'

import { LabelWithInfo } from './InfoTip'
import {
  buildRankingPortfolioDetailHref,
  buildRankingPortfolioChartSeriesData,
  formatRankingPortfolioCode,
  formatRankingPortfolioDate,
  formatRankingPortfolioDateTime,
  formatRankingPortfolioPercent,
  formatRankingPortfolioReferencePrice,
  formatRankingPortfolioWeight,
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
      <div className="flex min-h-[300px] items-center justify-center rounded-2xl border border-dashed border-border/70 bg-black/10 text-sm text-white/35 xl:h-full">
        暂无模拟组合曲线
      </div>
    )
  }

  return (
    <div className="overflow-hidden rounded-2xl border border-border/70 bg-[radial-gradient(circle_at_top_left,rgba(245,158,11,0.14),transparent_45%),linear-gradient(180deg,rgba(255,255,255,0.03),rgba(255,255,255,0.01))]">
      <div className="flex flex-wrap items-center gap-x-4 gap-y-2 border-b border-white/6 px-4 py-3 text-[11px] text-white/55">
        <ChartLegendItem colorClass="bg-amber-400" label="模拟组合累计收益" />
        <ChartLegendItem colorClass="bg-slate-300" label={`${benchmarkLabel}累计收益`} />
        <span className="text-white/30">横轴时间 · 纵轴累计收益</span>
      </div>

      <div className="relative px-2 pb-2 pt-3">
        <div ref={containerRef} className="h-[280px] w-full" role="img" aria-label="模拟组合与基准累计收益图表" />
        {tooltip ? (
          <div
            className="pointer-events-none absolute z-10 min-w-[180px] rounded-xl border border-white/10 bg-slate-950/94 px-3 py-2 shadow-2xl backdrop-blur"
            style={{ left: tooltip.left, top: tooltip.top }}
          >
            <div className="text-[11px] text-white/42">{tooltip.point.date}</div>
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

  useEffect(() => {
    if (!panelRef.current) return
  }, [])

  const meta = data?.meta || null
  const series = Array.isArray(data?.series) ? data.series : []
  const constituents = Array.isArray(data?.constituents) ? data.constituents : []
  const latestRebalance = data?.latest_rebalance && typeof data.latest_rebalance === 'object' ? data.latest_rebalance : null
  const benchmarkLabel = meta?.benchmark_name || meta?.benchmark_code || '上证指数'

  if (loading && !meta) {
    return (
      <section className="rounded-2xl border border-border bg-card p-5">
        <div className="flex h-56 items-center justify-center text-sm text-white/35">
          <span className="animate-pulse">加载模拟组合收益...</span>
        </div>
      </section>
    )
  }

  return (
    <section ref={panelRef} className="rounded-2xl border border-border bg-card p-5">
      <div className="flex flex-col gap-4 lg:flex-row lg:items-start lg:justify-between">
        <div className="min-w-0 flex-1">
          <div className="flex flex-wrap items-center gap-2">
            <h3 className="text-base font-semibold text-white">卧龙AI精选模拟组合</h3>
            <span className="rounded-full border border-amber-300/20 bg-amber-500/10 px-2 py-0.5 text-[11px] text-amber-200">收益质量跟踪</span>
          </div>
          <p className="mt-1 text-xs leading-5 text-white/45">{meta?.method_note || '模拟组合规则说明加载中。'}</p>
        </div>

        <div className="grid grid-cols-3 gap-3 lg:min-w-[280px]">
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

      <div className="mt-4 grid gap-4 xl:grid-cols-[minmax(0,1.5fr)_minmax(280px,0.8fr)] xl:items-stretch">
        <div className="xl:h-full">
          <RankingPortfolioChart series={series} benchmarkLabel={benchmarkLabel} />
        </div>

        <div className="rounded-2xl border border-border/70 bg-black/10 p-4 xl:h-full">
          <div className="flex items-center justify-between gap-3">
            <div>
              <div className="text-sm font-medium text-white">当前成分股</div>
            </div>
          </div>

          {meta?.has_shortfall ? (
            <div className="mt-3 rounded-xl border border-amber-300/20 bg-amber-500/10 px-3 py-2 text-xs text-amber-200">
              {meta?.warning_text || '当日有效成分股不足 4 只'}
            </div>
          ) : null}

          <div className="mt-3 space-y-2">
            {constituents.length ? constituents.map((item) => (
              <RankingPortfolioConstituentRow key={`${item.exchange}-${item.code}`} item={item} />
            )) : (
              <div className="rounded-xl border border-dashed border-border/70 px-4 py-8 text-center text-sm text-white/35">
                暂无成分股数据
              </div>
            )}
          </div>

          <LatestRebalanceDisclosure rebalance={latestRebalance} />
        </div>
      </div>
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
      <div className="inline-flex items-center gap-2 text-white/62">
        <span className={`h-2 w-2 rounded-full ${colorClass}`} aria-hidden="true" />
        <span>{label}</span>
      </div>
      <div className={highlight ? getRankingPortfolioPerformanceClass(value) : 'text-white'}>{formatRankingPortfolioPercent(value)}</div>
    </div>
  )
}

function MetricCard({ label, tooltip, value, valueClass }) {
  return (
    <div className="rounded-2xl border border-border/70 bg-black/10 px-3 py-3">
      <div className="text-[11px] text-white/35">
        <LabelWithInfo label={label} tooltip={tooltip} labelClassName="text-[11px] text-white/35" tipPlacement="top-right" tipWidthClassName="w-52 sm:w-60" />
      </div>
      <div className={`mt-2 text-lg font-semibold tabular-nums ${valueClass}`}>{value}</div>
    </div>
  )
}

function RankingPortfolioConstituentRow({ item }) {
  const detailHref = buildRankingPortfolioDetailHref(item?.code, item?.exchange)
  const content = (
    <>
      <div className="min-w-0">
        <div className="truncate text-sm font-medium text-white transition group-hover:text-amber-100">{item?.name || '--'}</div>
        <div className="mt-0.5 text-[11px] text-white/35">#{item?.rank} · {formatRankingPortfolioCode(item?.code, item?.exchange)}</div>
      </div>
      <div className="text-right">
        <div className="text-sm font-semibold text-white">{formatRankingPortfolioPercent((item?.weight || 0) * 100, 0)}</div>
        <div className="mt-0.5 text-[11px] text-white/35">仓位</div>
      </div>
    </>
  )

  if (!detailHref) {
    return <div className="flex items-center justify-between rounded-xl border border-white/5 bg-white/[0.03] px-3 py-2">{content}</div>
  }

  return (
    <a
      href={detailHref}
      target="_blank"
      rel="noreferrer"
      title={`查看 ${item?.name || formatRankingPortfolioCode(item?.code, item?.exchange)} 详情`}
      className="group flex items-center justify-between rounded-xl border border-white/5 bg-white/[0.03] px-3 py-2 transition hover:border-amber-300/30 hover:bg-amber-500/[0.06] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-amber-300/60"
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
      <summary className="inline-flex cursor-pointer list-none items-center gap-2 rounded-full border border-white/10 bg-black/15 px-2.5 py-1 text-[11px] text-white/52 transition hover:border-white/18 hover:text-white/72 marker:hidden">
        <span>最近一次调仓</span>
        <span className="rounded-full bg-white/8 px-1.5 py-0.5 text-[10px] text-white/45">{changeCount}项</span>
      </summary>

      <div className="mt-2 rounded-xl border border-white/8 bg-white/[0.02] px-3 py-3">
        <div className="text-[11px] leading-5 text-white/42">
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
          <div className="mt-3 rounded-xl border border-dashed border-border/70 px-3 py-4 text-center text-sm text-white/35">
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
    ? 'border-emerald-400/20 bg-emerald-500/10 text-emerald-200'
    : 'border-amber-300/20 bg-amber-500/10 text-amber-200'

  return (
    <div className="rounded-xl border border-white/5 bg-black/10 px-3 py-2.5">
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0">
          <div className="flex flex-wrap items-center gap-2">
            <span className={`rounded-full border px-2 py-0.5 text-[11px] ${badgeClass}`}>{getRankingPortfolioRebalanceActionLabel(item?.action)}</span>
            <span className="truncate text-sm font-medium text-white">{item?.name || '--'}</span>
          </div>
          <div className="mt-1 text-[11px] text-white/35">{formatRankingPortfolioCode(item?.code, item?.exchange)}</div>
        </div>

        <div className="text-right text-[11px] text-white/42">
          <div>仓位 {formatRankingPortfolioWeightChange(item?.from_weight, item?.to_weight, 0)}</div>
          <div className="mt-1">参考成本价 {formatRankingPortfolioReferencePrice(item?.reference_cost_price, item?.exchange)}</div>
        </div>
      </div>
    </div>
  )
}
