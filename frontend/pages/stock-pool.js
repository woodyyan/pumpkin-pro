import { useEffect, useMemo, useState } from 'react'

import Head from 'next/head'
import Link from 'next/link'
import { requestJson } from '../lib/api'
import { buildStockPoolState } from '../lib/stock-pool'

export default function StockPoolPage() {
  const [payload, setPayload] = useState(null)
  const [loaded, setLoaded] = useState(false)
  const [activeFactor, setActiveFactor] = useState('value')

  useEffect(() => {
    let cancelled = false
    const loadPool = async () => {
      try {
        const data = await requestJson('/api/live/factor-pool')
        if (!cancelled) setPayload(data)
      } catch {
        if (!cancelled) setPayload(null)
      } finally {
        if (!cancelled) setLoaded(true)
      }
    }
    loadPool()
    const intervalId = window.setInterval(loadPool, 10000)
    return () => {
      cancelled = true
      window.clearInterval(intervalId)
    }
  }, [])

  const state = useMemo(() => buildStockPoolState(payload), [payload])

  return (
    <div className="space-y-6">
      <Head>
        <title>卧龙股池 — 卧龙AI量化交易台</title>
        <meta name="description" content="卧龙股池展示七个单因子指数当前月度生效的 Top10 成分股与基础行情。" />
        <link rel="canonical" href="https://wolongtrader.top/stock-pool" />
      </Head>

      <section className="rounded-3xl border border-border bg-card px-5 py-5 md:px-6 md:py-6">
        <div className="flex flex-col gap-5 lg:flex-row lg:items-end lg:justify-between">
          <div className="max-w-3xl">
            <div className="text-xs font-medium uppercase tracking-[0.18em] text-foreground-dim">Factor pool</div>
            <h1 className="mt-2 text-2xl font-semibold tracking-tight text-foreground md:text-3xl">卧龙股池</h1>
            <div className="mt-4 flex flex-wrap gap-3 text-sm">
              <Link href="/live-trading" className="inline-flex items-center rounded-xl bg-primary px-4 py-2 font-medium text-black transition hover:opacity-90">查看单因子指数</Link>
              <Link href="/factor-lab" className="inline-flex items-center rounded-xl border border-border bg-[var(--color-bg-hover)] px-4 py-2 font-medium text-foreground transition hover:border-primary/40 hover:text-primary">进入因子实验室</Link>
            </div>
          </div>
          <div className="grid gap-3 sm:grid-cols-2 lg:min-w-[340px]">
            <MetaStat label="榜单数据日期" value={state.sourceTradeDate || '--'} />
            <MetaStat label={state.quoteStatus === 'live' ? '行情更新时间' : '行情状态'} value={state.quoteStatus === 'live' ? formatDateTime(state.quoteUpdatedAt) : '行情待恢复'} />
          </div>
        </div>
      </section>

      <section className="space-y-4">
        <div className="flex flex-col gap-3 sm:flex-row sm:items-end sm:justify-between">
          <div>
            <div className="text-xs font-medium uppercase tracking-[0.16em] text-foreground-dim">Current constituents</div>
            <h2 className="mt-1 text-xl font-semibold tracking-tight text-foreground">当前因子榜单</h2>
          </div>
        </div>

        <div className="flex gap-2 overflow-x-auto pb-1 md:hidden">
          {state.lists.map((list) => (
            <button
              key={list.factorKey}
              type="button"
              onClick={() => setActiveFactor(list.factorKey)}
              className={`shrink-0 rounded-full border px-3 py-1.5 text-xs font-medium transition ${activeFactor === list.factorKey ? 'border-primary bg-primary text-black' : 'border-border bg-card text-foreground-muted'}`}
            >
              {shortName(list.name)}
            </button>
          ))}
        </div>

        {!loaded && state.lists.length === 0 ? <PoolSkeleton /> : null}
        {loaded && state.lists.length === 0 ? <PoolEmpty /> : null}
        <div className="hidden gap-4 md:grid md:grid-cols-2 2xl:grid-cols-3">
          {state.lists.map((list) => <PoolList key={list.factorKey} list={list} priceLabel={state.priceLabel} />)}
        </div>
        <div className="space-y-3 md:hidden">
          {state.lists.filter((list) => list.factorKey === activeFactor).map((list) => <PoolList key={list.factorKey} list={list} priceLabel={state.priceLabel} compact />)}
        </div>
      </section>

      <p className="mx-auto max-w-4xl text-center text-xs leading-5 text-foreground-dim">因子分仅用于同一榜单内排序；榜单反映模型筛选结果，不构成任何投资建议。</p>
    </div>
  )
}

function PoolList({ list, priceLabel, compact = false }) {
  return (
    <article className="overflow-hidden rounded-3xl border border-border bg-card">
      <div className="border-b border-border px-4 py-4">
        <div className="flex items-start justify-between gap-3">
          <div>
            <h3 className="text-base font-semibold text-foreground">{list.name}</h3>
            <p className="mt-1 text-xs text-foreground-muted">当前成分股 {list.currentConstituentCount || 0} 只 · 展示 Top{list.items.length || 10}</p>
          </div>
          <span className={`inline-flex shrink-0 rounded-full px-2.5 py-1 text-[11px] font-medium ${list.statusToneClass}`}>{list.statusLabel}</span>
        </div>
        <div className="mt-3 flex flex-wrap gap-x-3 gap-y-1 text-[11px] text-foreground-dim">
          <span>信号日：{list.rebalanceDate || '--'}</span>
          <span>生效日：{list.effectiveStartDate || '--'}</span>
        </div>
      </div>

      {list.items.length > 0 ? (
        <div className="divide-y divide-border/70">
          {!compact ? <PoolColumnHeader priceLabel={priceLabel} /> : null}
          {list.items.map((item) => <PoolRow key={`${list.factorKey}-${item.code}`} item={item} priceLabel={priceLabel} compact={compact} />)}
        </div>
      ) : (
        <div className="px-4 py-8 text-center text-sm text-foreground-dim">当前暂无可展示成分股</div>
      )}
      {list.warningText ? <div className="border-t border-amber-500/20 bg-amber-500/5 px-4 py-3 text-xs leading-5 text-amber-700">{list.warningText}</div> : null}
    </article>
  )
}

function PoolColumnHeader({ priceLabel }) {
  return (
    <div className="grid grid-cols-[28px_minmax(0,1fr)_72px_68px_70px] gap-2 px-4 py-2 text-[10px] text-foreground-disabled">
      <span>排名</span><span>股票</span><span className="text-right">因子分</span><span className="text-right">{priceLabel}</span><span className="text-right">当日涨跌</span>
    </div>
  )
}

function PoolRow({ item, priceLabel, compact }) {
  const changeClass = performanceClass(item.quote.changeRate)
  const href = item.symbol ? `/live-trading/${item.symbol}` : '#'
  if (compact) {
    return (
      <Link href={href} className="block px-4 py-3 transition hover:bg-[var(--color-bg-hover)]">
        <div className="grid grid-cols-[28px_minmax(0,1fr)_66px_72px] items-center gap-2">
          <RankBadge rank={item.rank} />
          <div className="min-w-0"><div className="truncate text-sm font-medium text-foreground">{item.name}</div><div className="mt-0.5 truncate text-[11px] text-foreground-dim">{item.code}{item.industry ? ` · ${item.industry}` : ''}</div></div>
          <div className="text-right text-xs tabular-nums text-foreground-muted">{formatScore(item.factorScore)}</div>
          <div className={`text-right text-sm font-medium tabular-nums ${changeClass}`}>{formatPercent(item.quote.changeRate)}</div>
        </div>
        <div className="mt-2 flex items-center justify-between gap-3 pl-7 text-[11px] text-foreground-dim"><span>{priceLabel} {formatPrice(item.quote.lastPrice)}</span><span>成交额 {formatTurnover(item.quote.turnover)}</span></div>
      </Link>
    )
  }
  return (
    <Link href={href} className="group grid grid-cols-[28px_minmax(0,1fr)_72px_68px_70px] items-center gap-2 px-4 py-3 transition hover:bg-[var(--color-bg-hover)]">
      <RankBadge rank={item.rank} />
      <div className="min-w-0"><div className="truncate text-sm font-medium text-foreground group-hover:text-primary">{item.name}</div><div className="mt-0.5 truncate text-[11px] text-foreground-dim">{item.code}{item.industry ? ` · ${item.industry}` : ''}</div></div>
      <div className="text-right text-xs tabular-nums text-foreground-muted">{formatScore(item.factorScore)}</div>
      <div className="text-right text-sm font-medium tabular-nums text-foreground">{formatPrice(item.quote.lastPrice)}</div>
      <div className={`text-right text-sm font-medium tabular-nums ${performanceClass(item.quote.changeRate)}`}>{formatPercent(item.quote.changeRate)}</div>
    </Link>
  )
}

function RankBadge({ rank }) {
  const className = rank <= 3 ? 'bg-primary/15 text-primary' : 'bg-[var(--color-bg-hover)] text-foreground-muted'
  return <span className={`inline-flex h-6 w-6 items-center justify-center rounded-full text-[11px] font-medium tabular-nums ${className}`}>{rank || '--'}</span>
}

function MetaStat({ label, value }) {
  return <div className="rounded-2xl border border-border/80 bg-[var(--color-bg-hover)] px-4 py-3"><div className="text-xs text-foreground-dim">{label}</div><div className="mt-1 text-sm font-medium tabular-nums text-foreground">{value}</div></div>
}

function PoolSkeleton() {
  return <div className="grid gap-4 md:grid-cols-2 2xl:grid-cols-3">{Array.from({ length: 7 }).map((_, index) => <div key={index} className="h-[470px] animate-pulse rounded-3xl border border-border bg-card" />)}</div>
}

function PoolEmpty() {
  return <div className="rounded-3xl border border-dashed border-border bg-card px-5 py-10 text-center text-sm text-foreground-dim">暂未获取到卧龙股池结果，请在单因子指数计算完成后重试。</div>
}

function shortName(name) {
  return String(name || '').replace('因子指数', '')
}

function formatScore(value) {
  return Number.isFinite(Number(value)) ? Number(value).toFixed(1) : '--'
}

function formatPrice(value) {
  return Number.isFinite(Number(value)) ? Number(value).toFixed(2) : '--'
}

function formatPercent(value) {
  if (!Number.isFinite(Number(value))) return '--'
  const numeric = Number(value) * 100
  return `${numeric > 0 ? '+' : ''}${numeric.toFixed(2)}%`
}

function formatTurnover(value) {
  if (!Number.isFinite(Number(value)) || Number(value) <= 0) return '--'
  const numeric = Number(value)
  return numeric >= 10000 ? `${(numeric / 10000).toFixed(1)}亿` : `${numeric.toFixed(0)}万`
}

function performanceClass(value) {
  if (!Number.isFinite(Number(value))) return 'text-foreground-dim'
  return Number(value) >= 0 ? 'text-negative' : 'text-positive'
}

function formatDateTime(value) {
  if (!value) return '--'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return new Intl.DateTimeFormat('zh-CN', { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', hour12: false, timeZone: 'Asia/Shanghai' }).format(date)
}
