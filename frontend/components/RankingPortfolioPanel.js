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

function RankingPortfolioChart({ series = [] }) {
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

      const portfolioSeries = chart.addLineSeries({
        color: CHART_COLORS.portfolio,
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
  }, [chartData.baseline, chartData.points, chartData.portfolio, pointMap, resolvedTheme])

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
        <span className="text-foreground-dim">悬停查看单日收益与回撤</span>
      </div>

      <div className="relative px-2 pb-2 pt-3">
        <div ref={containerRef} className="h-[280px] w-full" role="img" aria-label="模拟组合累计收益图表" />
        {tooltip ? (
          <div
            className="pointer-events-none absolute z-10 min-w-[194px] rounded-xl border border-border bg-card/94 px-3 py-2 shadow-2xl backdrop-blur"
            style={{ left: tooltip.left, top: tooltip.top }}
          >
            <div className="text-[11px] text-foreground/42">{tooltip.point.sourceTradeDate ? `${tooltip.point.sourceTradeDate} 收盘` : tooltip.point.date}</div>
            <TooltipMetric label="累计收益" value={tooltip.point.portfolioReturnPct} colorClass="bg-amber-400" highlight />
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
  const [selectedExchange, setSelectedExchange] = useState('ASHARE')
  const [selectedVariant, setSelectedVariant] = useState('A')
  const [bannerDismissed, setBannerDismissed] = useState(false)
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
      {!bannerDismissed && (
        <div className="mb-4 flex items-start gap-3 rounded-xl border border-amber-300/40 dark:border-amber-300/20 bg-amber-100 dark:bg-amber-500/10 px-4 py-3">
          <div className="flex-1 min-w-0">
            <p className="text-sm font-medium text-amber-900 dark:text-amber-100">📣 模拟组合口径更新</p>
            <p className="mt-1 text-xs leading-5 text-amber-800/80 dark:text-amber-200/70">即日起，模拟买入价从"当日收盘价"切换为"次一交易日 9:25 集合竞价开盘价"，更贴近真实交易。收益曲线和成分股涨幅已按新口径重新计算，历史数据同步更新。</p>
          </div>
          <button
            type="button"
            onClick={() => setBannerDismissed(true)}
            className="shrink-0 rounded-lg p-1 text-amber-600 hover:bg-amber-200/50 dark:text-amber-300 dark:hover:bg-amber-500/20"
            aria-label="关闭公告"
          >
            <svg width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round">
              <path d="M4 4l8 8M12 4l-8 8" />
            </svg>
          </button>
        </div>
      )}
      <div className="flex flex-col gap-3 lg:flex-row lg:items-start lg:justify-between">
        <div className="min-w-0 flex-1">
          <div className="flex flex-wrap items-center gap-2">
            <h3 className="text-base font-semibold text-foreground">卧龙AI精选模拟组合</h3>
            <span className="rounded-full border border-amber-300/40 dark:border-amber-300/20 bg-amber-100 dark:bg-amber-500/10 px-2 py-0.5 text-[11px] font-medium text-amber-800 dark:text-amber-200">A / B 双组合</span>
          </div>
          <p className="mt-1 text-xs leading-5 text-foreground-dim">跟踪卧龙AI精选 A、B 两套组合的开盘价模拟表现：收盘后选股，次一交易日开盘价买入、当日收盘价结算，核心看累计收益、回撤、波动与日胜率。</p>
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
            <div className="flex flex-col gap-3">
              <div className="flex flex-wrap items-center gap-2 text-[11px] text-foreground-dim">
                <span className="rounded-full border border-border bg-[var(--color-bg-hover)] px-2 py-1 text-foreground-muted">{selectedExchange === 'HKEX' ? '港股' : 'A股'} {meta?.name || '模拟组合'}</span>
                <span className="rounded-full border border-border bg-[var(--color-bg-hover)] px-2 py-1">{selectionSummary}</span>
                {meta?.inception_trade_date ? <span className="rounded-full border border-border bg-[var(--color-bg-hover)] px-2 py-1">起始日 {meta.inception_trade_date}</span> : null}
              </div>

              <div className="grid grid-cols-2 gap-2 md:grid-cols-4 xl:grid-cols-7">
                <MetricCard
                  label="累计收益"
                  tooltip="模拟组合从成立以来持有到当前的总收益率。"
                  value={formatRankingPortfolioPercent(meta?.latest_portfolio_return_pct)}
                  valueClass={getRankingPortfolioPerformanceClass(meta?.latest_portfolio_return_pct)}
                />
                <MetricCard
                  label="成立天数"
                  tooltip="按起始收盘日到最新收盘日的自然日跨度统计，帮助用户判断累计收益的观察区间。"
                  value={formatRankingPortfolioDayCount(meta?.inception_days)}
                  valueClass={getNeutralMetricClass(meta?.inception_days)}
                  subtext={meta?.inception_trade_date ? `起始日 ${meta.inception_trade_date}` : ''}
                />
                <MetricCard
                  label="昨日收益率"
                  tooltip="上一交易日相对前一持仓日收盘价的组合收益率，含调仓成本影响。"
                  value={formatRankingPortfolioPercent(meta?.latest_daily_return_pct)}
                  valueClass={getRankingPortfolioPerformanceClass(meta?.latest_daily_return_pct)}
                />
                <MetricCard
                  label="本月收益率"
                  tooltip="自然月内按收盘价口径统计的组合收益率；若成立未满一个月，则与累计收益率一致。"
                  value={formatRankingPortfolioPercent(meta?.current_month_return_pct)}
                  valueClass={getRankingPortfolioPerformanceClass(meta?.current_month_return_pct)}
                />
                <MetricCard
                  label="最大回撤"
                  tooltip="成立以来从任一历史高点回落的最大跌幅，用于衡量最差持有体验。"
                  value={formatRankingPortfolioPercent(meta?.max_drawdown_pct)}
                  valueClass={getNeutralMetricClass(meta?.max_drawdown_pct)}
                />
                <MetricCard
                  label="波动率"
                  tooltip="成立以来日收益率的年化波动率，反映组合日度起伏大小。"
                  value={formatRankingPortfolioPercent(meta?.volatility_pct)}
                  valueClass={getNeutralMetricClass(meta?.volatility_pct)}
                />
                <MetricCard
                  label="日胜率"
                  tooltip="成立以来盈利交易日占比，按正收益交易日数除以全部有收益记录的交易日数计算。"
                  value={formatRankingPortfolioPercent(meta?.daily_win_rate_pct)}
                  valueClass={getWinRateClass(meta?.daily_win_rate_pct)}
                />
              </div>
            </div>
          </div>

          <div className="mt-3 grid gap-3 xl:grid-cols-[minmax(0,1.52fr)_minmax(290px,0.86fr)] xl:items-stretch">
            <div className="xl:h-full">
              <RankingPortfolioChart series={series} />
            </div>

            <div className="rounded-2xl border border-border/70 bg-[var(--color-bg-hover)] p-3.5 xl:h-full">
              <div className="text-sm font-medium text-foreground">当前成分股</div>
              {currentConstituentHint ? <div className="mt-1 text-[11px] leading-5 text-foreground/56 dark:text-foreground/42">{currentConstituentHint}</div> : null}
              {meta?.source_trade_date ? <div className="mt-1 text-[11px] leading-5 text-foreground/56 dark:text-foreground/42">收益曲线截至：{formatCloseDateLabel(meta?.source_trade_date, meta?.ranking_time)}，按开盘价模拟买入、收盘价模拟调仓</div> : null}
              {meta?.is_same_batch_as_performance === false && meta?.batch_mismatch_reason ? <div className="mt-2 rounded-xl border border-sky-400/35 bg-sky-100 text-sky-700 dark:border-sky-300/20 dark:bg-sky-500/10 px-3 py-2 text-xs dark:text-sky-200">{meta.batch_mismatch_reason}</div> : null}

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

function RankingPortfolioConstituentRow({ item, showSourceRank = false }) {
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
    return <div className="flex items-center justify-between rounded-xl border border-border bg-[var(--color-bg-hover)] px-3 py-2">{content}</div>
  }

  return (
    <a
      href={detailHref}
      target="_blank"
      rel="noreferrer"
      title={`查看 ${item?.name || formatRankingPortfolioCode(item?.code, item?.exchange)} 详情`}
      className="group flex items-center justify-between rounded-xl border border-border bg-[var(--color-bg-hover)] px-3 py-2 transition hover:border-primary/30 dark:hover:border-amber-300/30 hover:bg-primary/[0.04] dark:hover:bg-amber-500/[0.06] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary/60 dark:focus-visible:ring-amber-300/60"
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
    ? 'border-emerald-400/35 bg-emerald-100 text-emerald-700 dark:border-positive/20 dark:bg-positive/10 dark:text-positive'
    : 'border-amber-400/40 bg-amber-100 text-amber-700 dark:border-amber-300/20 dark:bg-amber-500/10 dark:text-amber-200'

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
