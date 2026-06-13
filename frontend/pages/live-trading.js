import { useEffect, useMemo, useState } from 'react'

import Head from 'next/head'
import Link from 'next/link'
import MiniChart from '../components/MiniChart'
import { requestJson } from '../lib/api'
import {
  buildMarketState,
  formatNumber,
  formatPercent,
  formatSignedNumber,
  formatTime,
} from '../lib/live-trading-market'

export default function LiveTradingOverviewPage() {
  const [marketOverviewA, setMarketOverviewA] = useState(null)
  const [marketOverviewHK, setMarketOverviewHK] = useState(null)
  const [hasLoaded, setHasLoaded] = useState(false)
  const [lastUpdatedAt, setLastUpdatedAt] = useState(null)

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
        setLastUpdatedAt(new Date())
        setHasLoaded(true)
      } catch {
        if (!cancelled) {
          setHasLoaded(true)
        }
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

  const marketState = useMemo(() => buildMarketState(marketOverviewA, marketOverviewHK), [marketOverviewA, marketOverviewHK])

  return (
    <div className="space-y-6">
      <Head>
        <title>市场行情 — 卧龙AI量化交易台</title>
        <meta name="description" content="卧龙AI量化交易台市场行情页，集中查看 A 股与港股大盘指数。个股关注与实时卡片已迁移到自选股页面。" />
        <link rel="canonical" href="https://wolongtrader.top/live-trading" />
      </Head>

      <section className="overflow-hidden rounded-3xl border border-border bg-card px-5 py-5 md:px-6 md:py-6">
        <div className="flex flex-col gap-5 lg:flex-row lg:items-end lg:justify-between">
          <div className="max-w-3xl">
            <div className="text-xs font-medium uppercase tracking-[0.18em] text-foreground-dim">Market</div>
            <h1 className="mt-2 text-2xl font-semibold tracking-tight text-foreground md:text-3xl">市场行情</h1>
            <p className="mt-2 text-sm leading-6 text-foreground-muted">
              这里专注展示大盘指数。关注股票、实时卡片和进入个股详情的入口已迁移到自选股页面。
            </p>
            <div className="mt-3 flex flex-wrap items-center gap-2 text-xs text-foreground-dim">
              <span className="inline-flex rounded-full border border-border px-2.5 py-1">{marketState.trendSummary || '展示后端返回的真实指数趋势序列'}</span>
              {marketState.updatedAt ? <span>行情时间 {formatTime(marketState.updatedAt)}</span> : null}
            </div>
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
                <div className="mt-1 text-xs leading-5 text-foreground-muted">{item.description}</div>
              </div>
            ))}
          </div>
        </div>
      </section>

      <section className="space-y-4">
        <div className="flex flex-wrap items-end justify-between gap-3">
          <div>
            <div className="text-xs font-medium uppercase tracking-[0.16em] text-foreground-dim">Core Indexes</div>
            <h2 className="mt-1 text-xl font-semibold tracking-tight text-foreground">核心指数卡片</h2>
            <p className="mt-1 text-sm text-foreground-muted">首屏保留 A 股与港股各自最重要的宽基与科技主线，直接展示关键指数卡片。</p>
          </div>
          <div className="text-xs text-foreground-dim">
            {lastUpdatedAt ? `最近刷新 ${formatTime(lastUpdatedAt)}` : !hasLoaded ? '加载中...' : '等待行情刷新'}
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
          <p className="mt-1 text-sm text-foreground-muted">用少量文字解释今天的指数强弱分布，而不是只堆行情数值。</p>
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
      <div className="mt-3 flex items-center justify-between text-xs text-foreground-dim">
        <span>真实趋势</span>
        <span>{index.chartMeta?.pointCount || index.trend.length} 点</span>
      </div>
    </article>
  )
}

function MarketCardSkeleton() {
  return <div className="h-[250px] animate-pulse rounded-3xl border border-border bg-card" />
}
