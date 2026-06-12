import { useEffect, useMemo, useState } from 'react'

import Head from 'next/head'

import QuadrantOverviewSection from '../components/QuadrantOverviewSection'
import RankingOverviewSection from '../components/RankingOverviewSection'
import { requestJson } from '../lib/api'
import { useAuth } from '../lib/auth-context'

export default function QuadrantPage() {
  const { isLoggedIn, ready } = useAuth()
  const [watchlistSymbols, setWatchlistSymbols] = useState([])

  useEffect(() => {
    if (!ready) return undefined
    if (!isLoggedIn) {
      setWatchlistSymbols([])
      return undefined
    }

    let cancelled = false

    const loadWatchlist = async () => {
      try {
        const data = await requestJson('/api/live/watchlist')
        if (cancelled) return
        const symbols = Array.isArray(data?.items)
          ? data.items.map((item) => String(item?.symbol || '').trim()).filter(Boolean)
          : []
        setWatchlistSymbols(symbols)
      } catch {
        if (!cancelled) {
          setWatchlistSymbols([])
        }
      }
    }

    loadWatchlist()
    return () => {
      cancelled = true
    }
  }, [isLoggedIn, ready])

  const watchlistCount = useMemo(() => watchlistSymbols.length, [watchlistSymbols])

  return (
    <div className="space-y-6">
      <Head>
        <title>四象限与卧龙AI精选 — 卧龙AI量化交易台</title>
        <meta name="description" content="查看全市场风险机会全景图与卧龙AI精选榜单。支持 A 股 / 港股切换、搜索定位股票，并在登录后高亮我的关注。" />
        <link rel="canonical" href="https://wolongtrader.top/quadrant" />
      </Head>

      <section className="rounded-2xl border border-border bg-card px-5 py-5">
        <div className="flex flex-col gap-3 md:flex-row md:items-end md:justify-between">
          <div>
            <div className="text-xs font-medium uppercase tracking-[0.18em] text-foreground-dim">Quadrant</div>
            <h1 className="mt-2 text-2xl font-semibold tracking-tight text-foreground md:text-3xl">风险机会全景图与卧龙AI精选</h1>
            <p className="mt-2 max-w-3xl text-sm leading-6 text-foreground-muted">
              先看全市场四象限分布，再下钻到卧龙AI精选榜单。A 股与港股分市场查看，登录后会额外高亮你的关注股票。
            </p>
          </div>
          <div className="flex flex-wrap gap-2 text-xs text-foreground-dim">
            <span className="rounded-full border border-border bg-[var(--color-bg-hover)] px-3 py-1.5">A 股 / 港股双市场</span>
            <span className="rounded-full border border-border bg-[var(--color-bg-hover)] px-3 py-1.5">支持搜索定位</span>
            <span className="rounded-full border border-border bg-[var(--color-bg-hover)] px-3 py-1.5">{isLoggedIn ? `已同步 ${watchlistCount} 只关注股票` : '登录后高亮我的关注'}</span>
          </div>
        </div>
      </section>

      <QuadrantOverviewSection watchlistSymbols={watchlistSymbols} />
      <RankingOverviewSection />
    </div>
  )
}
