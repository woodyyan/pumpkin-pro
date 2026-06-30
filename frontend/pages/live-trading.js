import { useEffect, useMemo, useState } from 'react'

import Head from 'next/head'
import Link from 'next/link'
import MiniChart from '../components/MiniChart'
import { requestJson } from '../lib/api'
import { buildFactorIndexState } from '../lib/live-factor-index'
import {
  buildMarketState,
  formatNumber,
  formatPercent,
  formatSignedNumber,
} from '../lib/live-trading-market'

export default function LiveTradingOverviewPage() {
  const [marketOverviewA, setMarketOverviewA] = useState(null)
  const [marketOverviewHK, setMarketOverviewHK] = useState(null)
  const [factorIndexOverview, setFactorIndexOverview] = useState(null)
  const [hasLoaded, setHasLoaded] = useState(false)
  const [factorIndexLoaded, setFactorIndexLoaded] = useState(false)

  useEffect(() => {
    let cancelled = false

    const loadMarketOverview = async () => {
      try {
        const [aRes, hkRes] = await Promise.allSettled([
          requestJson('/api/live/market/overview?exchange=SSE'),
          requestJson('/api/live/market/overview'),
        ])

        if (cancelled) return
        if (aRes.status === 'fulfilled') setMarketOverviewA(aRes.value)
        if (hkRes.status === 'fulfilled') setMarketOverviewHK(hkRes.value)
        setHasLoaded(true)
      } catch {
        if (!cancelled) setHasLoaded(true)
      }
    }

    loadMarketOverview()
    const intervalId = window.setInterval(() => {
      loadMarketOverview()
    }, 10000)

    return () => {
      cancelled = true
      window.clearInterval(intervalId)
    }
  }, [])

  useEffect(() => {
    let cancelled = false

    const loadFactorIndexOverview = async () => {
      try {
        const payload = await requestJson('/api/live/factor-index/overview')
        if (!cancelled) {
          setFactorIndexOverview(payload)
          setFactorIndexLoaded(true)
        }
      } catch {
        if (!cancelled) setFactorIndexLoaded(true)
      }
    }

    loadFactorIndexOverview()

    return () => {
      cancelled = true
    }
  }, [])

  const marketState = useMemo(() => buildMarketState(marketOverviewA, marketOverviewHK), [marketOverviewA, marketOverviewHK])
  const factorIndexState = useMemo(() => buildFactorIndexState(factorIndexOverview), [factorIndexOverview])

  return (
    <div className="space-y-6">
      <Head>
        <title>市场行情 — 卧龙AI量化交易台</title>
        <meta name="description" content="卧龙AI量化交易台市场行情页，集中查看单因子指数、A 股与港股大盘指数。个股关注与实时卡片已迁移到自选股页面。" />
        <link rel="canonical" href="https://wolongtrader.top/live-trading" />
      </Head>

      <section className="overflow-hidden rounded-3xl border border-border bg-card px-5 py-5 md:px-6 md:py-6">
        <div className="flex flex-col gap-5 lg:flex-row lg:items-end lg:justify-between">
          <div className="max-w-3xl">
            <div className="text-xs font-medium uppercase tracking-[0.18em] text-foreground-dim">Market</div>
            <h1 className="mt-2 text-2xl font-semibold tracking-tight text-foreground md:text-3xl">市场行情</h1>
            <div className="mt-4 flex flex-wrap gap-3 text-sm">
              <Link href="/watchlist" className="inline-flex items-center rounded-xl bg-primary px-4 py-2 font-medium text-black transition hover:opacity-90">
                去自选股
              </Link>
              <Link href="/quadrant" className="inline-flex items-center rounded-xl border border-border bg-[var(--color-bg-hover)] px-4 py-2 font-medium text-foreground transition hover:border-primary/40 hover:text-primary">
                看四象限
              </Link>
            </div>
          </div>
          <div className="grid gap-3 sm:grid-cols-3 lg:min-w-[420px] lg:max-w-[540px] lg:flex-1">
            {marketState.heroStats.map((item) => (
              <div key={item.label} className="rounded-2xl border border-border/80 bg-[var(--color-bg-hover)] px-4 py-4">
                <div className="text-xs font-medium uppercase tracking-[0.14em] text-foreground-dim">{item.label}</div>
                <div className="mt-2 text-lg font-semibold text-foreground">{item.value}</div>
              </div>
            ))}
          </div>
        </div>
      </section>

      <section className="space-y-4">
        <div className="flex flex-wrap items-end justify-between gap-3">
          <div>
            <div className="text-xs font-medium uppercase tracking-[0.16em] text-foreground-dim">Factor indexes</div>
            <h2 className="mt-1 text-xl font-semibold tracking-tight text-foreground">单因子指数</h2>
            <p className="mt-1 text-sm text-foreground-muted">Top50 等权、月度调仓、净值基准 1000。</p>
          </div>
          <div className="text-sm text-foreground-muted">
            数据基于：{factorIndexState.sourceTradeDate || '--'}
          </div>
        </div>
        <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-3 2xl:grid-cols-4">
          {factorIndexState.items.map((item) => (
            <FactorIndexCard key={item.factorKey} item={item} />
          ))}
          {!factorIndexLoaded && factorIndexState.items.length === 0 && Array.from({ length: 7 }).map((_, idx) => <FactorIndexSkeleton key={idx} />)}
        </div>
        {factorIndexLoaded && factorIndexState.items.length === 0 && (
          <div className="rounded-2xl border border-dashed border-border px-4 py-5 text-sm text-foreground-dim">
            暂未获取到单因子指数结果，请稍后重试。
          </div>
        )}
      </section>

      <section className="space-y-4">
        <div className="flex flex-wrap items-end justify-between gap-3">
          <div>
            <div className="text-xs font-medium uppercase tracking-[0.16em] text-foreground-dim">Core Indexes</div>
            <h2 className="mt-1 text-xl font-semibold tracking-tight text-foreground">核心指数卡片</h2>
          </div>
        </div>
        <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
          {marketState.coreIndexes.map((index) => (
            <MarketIndexCard key={index.code} index={index} />
          ))}
          {!hasLoaded && marketState.coreIndexes.length === 0 && Array.from({ length: 6 }).map((_, idx) => <MarketCardSkeleton key={idx} />)}
        </div>
      </section>

      <section className="space-y-4 rounded-3xl border border-border bg-card px-5 py-5">
        <div>
          <div className="text-xs font-medium uppercase tracking-[0.16em] text-foreground-dim">Market Insights</div>
          <h2 className="mt-1 text-xl font-semibold tracking-tight text-foreground">市场观察摘要</h2>
        </div>
        <div className="grid gap-3 lg:grid-cols-3">
          {marketState.insights.map((item) => (
            <div key={item.title} className="rounded-2xl border border-border/80 bg-[var(--color-bg-hover)] px-4 py-4">
              <div className="flex items-center justify-between gap-3">
                <div className="text-sm font-medium text-foreground">{item.title}</div>
                <span className={`inline-flex rounded-full px-2.5 py-1 text-[11px] font-medium ${item.accentClass}`}>
                  {item.tag}
                </span>
              </div>
              <p className="mt-2 text-sm leading-6 text-foreground-muted">{item.description}</p>
            </div>
          ))}
        </div>
        {hasLoaded && marketState.coreIndexes.length === 0 && (
          <div className="rounded-2xl border border-dashed border-border px-4 py-5 text-sm text-foreground-dim">
            暂未获取到指数行情，请稍后重试。
          </div>
        )}
      </section>
    </div>
  )
}

function FactorIndexCard({ item }) {
  const accentClass = getPerformanceClass(item.dailyReturn)
  const chartColor = item.dailyReturn >= 0 ? '#ef4444' : '#22c55e'
  const chartAreaColor = item.dailyReturn >= 0 ? 'rgba(239,68,68,0.16)' : 'rgba(34,197,94,0.16)'

  return (
    <article className="overflow-hidden rounded-3xl border border-border bg-card px-5 py-5 text-left shadow-[0_10px_30px_rgba(15,23,42,0.08)]">
      <div className="flex items-start justify-between gap-3">
        <div>
          <div className="text-base font-semibold text-foreground">{item.name}</div>
          <div className="mt-1 text-xs text-foreground-muted">当前成分股 {item.constituentCount || 0} 只</div>
        </div>
        <span className={`inline-flex rounded-full px-2.5 py-1 text-[11px] font-medium ${item.statusToneClass}`}>
          {item.statusLabel}
        </span>
      </div>

      <div className="mt-5 flex items-end justify-between gap-3">
        <div>
          <div className="text-2xl font-semibold tabular-nums text-foreground">{item.nav === null || item.nav === undefined ? '--' : formatNumber(item.nav, 2)}</div>
          <div className={`mt-1 text-sm font-medium tabular-nums ${accentClass}`}>{formatPercent(item.dailyReturn)}</div>
        </div>
        <div className="text-right text-xs text-foreground-dim">
          <div>调仓信号日</div>
          <div className="mt-1 tabular-nums text-foreground-muted">{item.rebalanceDate || '--'}</div>
        </div>
      </div>

      <div className="mt-4 grid grid-cols-3 gap-3 rounded-2xl border border-border/70 bg-[var(--color-bg-hover)] px-3 py-3 text-xs">
        <MetricBlock label="近1月" value={formatPercent(item.monthlyReturn)} toneClass={getPerformanceClass(item.monthlyReturn)} />
        <MetricBlock label="近3月" value={formatPercent(item.threeMonthReturn)} toneClass={getPerformanceClass(item.threeMonthReturn)} />
        <MetricBlock label="半年" value={formatPercent(item.halfYearReturn)} toneClass={getPerformanceClass(item.halfYearReturn)} />
      </div>

      <div className="mt-4 overflow-hidden rounded-2xl border border-border/70 bg-[var(--color-bg-hover)] px-3 py-3">
        {item.trend.length >= 2 ? (
          <MiniChart
            data={item.trend}
            label="近 20 日净值"
            width={320}
            height={120}
            color={chartColor}
            areaColor={chartAreaColor}
          />
        ) : (
          <div className="flex h-[120px] items-center justify-center text-sm text-foreground-dim">净值曲线生成中</div>
        )}
      </div>

      <div className="mt-3 flex items-start justify-between gap-3 text-xs text-foreground-dim">
        <div>数据日期：{item.sourceTradeDate || '--'}</div>
        <div>生效日：{item.effectiveStartDate || '--'}</div>
      </div>
      {item.warningText ? <div className="mt-2 text-xs text-amber-700">{item.warningText}</div> : null}
    </article>
  )
}

function MetricBlock({ label, value, toneClass }) {
  return (
    <div>
      <div className="text-[11px] text-foreground-dim">{label}</div>
      <div className={`mt-1 font-medium tabular-nums ${toneClass}`}>{value}</div>
    </div>
  )
}

function MarketIndexCard({ index }) {
  const accentClass = index.changeRate >= 0 ? 'text-negative' : 'text-positive'
  const chartColor = index.changeRate >= 0 ? '#ef4444' : '#22c55e'
  const chartAreaColor = index.changeRate >= 0 ? 'rgba(239,68,68,0.16)' : 'rgba(34,197,94,0.16)'

  return (
    <article
      className="overflow-hidden rounded-3xl border border-border bg-card px-5 py-5 text-left shadow-[0_10px_30px_rgba(15,23,42,0.08)] transition hover:border-primary/40"
      title={`${index.title} ${formatPercent(index.changeRate)}`}
    >
      <div className="flex items-start justify-between gap-3">
        <div>
          <div className="flex items-center gap-2">
            <span className="text-base font-semibold text-foreground">{index.title}</span>
            <span className="inline-flex rounded-full border border-border px-2 py-0.5 text-[11px] text-foreground-dim">{index.market}</span>
          </div>
          <div className="mt-1 text-xs text-foreground-muted">{index.description}</div>
        </div>
        <span className="rounded-full bg-[var(--color-bg-hover)] px-2.5 py-1 text-[11px] font-medium text-foreground-dim">{index.importance}</span>
      </div>

      <div className="mt-5 flex items-end justify-between gap-3">
        <div>
          <div className="text-2xl font-semibold tabular-nums text-foreground">{formatNumber(index.last, 2)}</div>
          <div className={`mt-1 text-sm font-medium tabular-nums ${accentClass}`}>{formatPercent(index.changeRate)}</div>
        </div>
        <div className={`text-right text-xs tabular-nums ${accentClass}`}>
          <div>{formatSignedNumber(index.changeAmount, 2)}</div>
          <div className="mt-1 text-foreground-dim">{index.pointLabel}</div>
        </div>
      </div>

      <div className="mt-4 overflow-hidden rounded-2xl border border-border/70 bg-[var(--color-bg-hover)] px-3 py-3">
        <MiniChart
          data={index.trend}
          label={index.chartMeta?.label || '走势'}
          width={320}
          height={120}
          color={chartColor}
          areaColor={chartAreaColor}
        />
      </div>
    </article>
  )
}

function FactorIndexSkeleton() {
  return <div className="h-[320px] animate-pulse rounded-3xl border border-border bg-card" />
}

function MarketCardSkeleton() {
  return <div className="h-[250px] animate-pulse rounded-3xl border border-border bg-card" />
}

function getPerformanceClass(value) {
  if (value === null || value === undefined || Number.isNaN(Number(value))) return 'text-foreground-dim'
  return Number(value) >= 0 ? 'text-negative' : 'text-positive'
}
