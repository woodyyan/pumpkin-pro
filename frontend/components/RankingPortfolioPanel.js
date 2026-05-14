import { useEffect, useRef } from 'react'

import { LabelWithInfo } from './InfoTip'
import {
  buildRankingPortfolioChartPoints,
  formatRankingPortfolioCode,
  formatRankingPortfolioPercent,
  getRankingPortfolioPerformanceClass,
} from '../lib/ranking-portfolio'

function RankingPortfolioChart({ series = [], benchmarkLabel = '上证指数' }) {
  const width = 720
  const height = 240
  const padding = 18
  const { portfolio, benchmark, baselineY } = buildRankingPortfolioChartPoints(series, width, height, padding)

  if (!series.length) {
    return (
      <div className="flex min-h-[260px] items-center justify-center rounded-2xl border border-dashed border-border/70 bg-black/10 text-sm text-white/35 xl:h-full">
        暂无模拟组合曲线
      </div>
    )
  }

  return (
    <div className="min-h-[260px] overflow-hidden rounded-2xl border border-border/70 bg-[radial-gradient(circle_at_top_left,rgba(245,158,11,0.14),transparent_45%),linear-gradient(180deg,rgba(255,255,255,0.03),rgba(255,255,255,0.01))] xl:h-full">
      <div className="flex flex-wrap items-center gap-x-4 gap-y-2 border-b border-white/6 px-4 py-3 text-[11px] text-white/55">
        <ChartLegendItem colorClass="bg-amber-400" label="模拟组合累计收益" />
        <ChartLegendItem colorClass="bg-slate-300" label={`${benchmarkLabel}累计收益`} />
      </div>
      <svg viewBox={`0 0 ${width} ${height}`} className="h-full min-h-[260px] w-full" role="img" aria-label="模拟组合与上证指数收益曲线" preserveAspectRatio="none">
        <line x1="0" x2={width} y1={baselineY} y2={baselineY} stroke="rgba(255,255,255,0.12)" strokeDasharray="4 5" />
        <path d={benchmark} fill="none" stroke="rgba(148,163,184,0.95)" strokeWidth="2.2" />
        <path d={portfolio} fill="none" stroke="rgba(245,158,11,1)" strokeWidth="2.8" />
      </svg>
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
              <div key={`${item.exchange}-${item.code}`} className="flex items-center justify-between rounded-xl border border-white/5 bg-white/[0.03] px-3 py-2">
                <div className="min-w-0">
                  <div className="truncate text-sm font-medium text-white">{item.name}</div>
                  <div className="mt-0.5 text-[11px] text-white/35">#{item.rank} · {formatRankingPortfolioCode(item.code, item.exchange)}</div>
                </div>
                <div className="text-right">
                  <div className="text-sm font-semibold text-white">{formatRankingPortfolioPercent((item.weight || 0) * 100, 0)}</div>
                  <div className="mt-0.5 text-[11px] text-white/35">平权</div>
                </div>
              </div>
            )) : (
              <div className="rounded-xl border border-dashed border-border/70 px-4 py-8 text-center text-sm text-white/35">
                暂无成分股数据
              </div>
            )}
          </div>
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
