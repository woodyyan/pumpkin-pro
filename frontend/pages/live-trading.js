import { useEffect, useState } from 'react'

import Head from 'next/head'
import Link from 'next/link'
import { requestJson } from '../lib/api'

export default function LiveTradingOverviewPage() {
  const [marketOverviewA, setMarketOverviewA] = useState(null)
  const [marketOverviewHK, setMarketOverviewHK] = useState(null)
  const [hasLoaded, setHasLoaded] = useState(false)

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

  const indexes = [...(marketOverviewA?.indexes || []), ...(marketOverviewHK?.indexes || [])]

  return (
    <div className="space-y-6">
      <Head>
        <title>市场行情 — 卧龙AI量化交易台</title>
        <meta name="description" content="卧龙AI量化交易台市场行情页，集中查看 A 股与港股大盘指数。个股关注与实时卡片已迁移到自选股页面。" />
        <link rel="canonical" href="https://wolongtrader.top/live-trading" />
      </Head>

      <section className="rounded-2xl border border-border bg-card px-5 py-5">
        <div className="max-w-3xl">
          <div className="text-xs font-medium uppercase tracking-[0.18em] text-foreground-dim">Market</div>
          <h1 className="mt-2 text-2xl font-semibold tracking-tight text-foreground md:text-3xl">市场行情</h1>
          <p className="mt-2 text-sm leading-6 text-foreground-muted">
            这里专注展示大盘指数。关注股票、实时卡片和进入个股详情的入口已迁移到自选股页面。
          </p>
          <div className="mt-4 flex flex-wrap gap-3 text-sm">
            <Link href="/watchlist" className="inline-flex items-center rounded-xl bg-primary px-4 py-2 font-medium text-black transition hover:opacity-90">
              去自选股
            </Link>
            <Link href="/quadrant" className="inline-flex items-center rounded-xl border border-border bg-[var(--color-bg-hover)] px-4 py-2 font-medium text-foreground transition hover:border-primary/40 hover:text-primary">
              看四象限
            </Link>
          </div>
        </div>
      </section>

      <section className="rounded-2xl border border-border bg-card px-5 py-4">
        <div className="flex flex-wrap items-center gap-x-5 gap-y-3">
          <span className="text-xs font-medium text-foreground-dim">大盘</span>
          {indexes.map((index) => (
            <div key={index.code} className="flex items-baseline gap-1.5">
              <span className="text-xs text-foreground-dim">{formatMarketIndexTitle(index.name, index.code)}</span>
              <span className="text-sm font-semibold tabular-nums text-foreground">{formatNumber(index.last, 2)}</span>
              <span className={`text-xs font-medium tabular-nums ${index.change_rate >= 0 ? 'text-negative' : 'text-positive'}`}>
                {formatPercent(index.change_rate)}
              </span>
            </div>
          ))}
          {!hasLoaded && indexes.length === 0 && (
            <span className="text-xs text-foreground-dim animate-pulse">加载中...</span>
          )}
          {hasLoaded && indexes.length === 0 && (
            <span className="text-xs text-foreground-dim">暂未获取到指数行情，请稍后重试。</span>
          )}
        </div>
      </section>
    </div>
  )
}

function formatPercent(value) {
  if (value === null || value === undefined || Number.isNaN(Number(value))) return '--'
  const num = Number(value) * 100
  const sign = num > 0 ? '+' : ''
  return `${sign}${num.toFixed(2)}%`
}

function formatNumber(value, digits = 2) {
  if (value === null || value === undefined || Number.isNaN(Number(value))) return '--'
  return Number(value).toLocaleString('zh-CN', { maximumFractionDigits: digits, minimumFractionDigits: digits })
}

function formatMarketIndexTitle(name, code) {
  const rawName = String(name || '').trim()
  const upperCode = String(code || '').trim().toUpperCase()
  const nameMap = {
    'Hang Seng Index': '恒生指数',
    'Hang Seng China Enterprises Index': '恒生中国企业指数',
    'Hang Seng TECH Index': '恒生科技指数',
  }
  if (nameMap[rawName]) return nameMap[rawName]
  const codeMap = {
    HSI: '恒生指数',
    HSCEI: '恒生中国企业指数',
    HSTECH: '恒生科技指数',
    '000001': '上证指数',
    '399001': '深证成指',
    '399006': '创业板指',
  }
  return codeMap[upperCode] || rawName || upperCode || '--'
}
