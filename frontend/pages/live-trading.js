import { useCallback, useEffect, useMemo, useState } from 'react'

import dynamic from 'next/dynamic'
import Head from 'next/head'
import QuadrantSearchBox from '../components/QuadrantSearchBox'
import { requestJson } from '../lib/api'
import { useAuth } from '../lib/auth-context'
import { isAuthRequiredError } from '../lib/auth-storage'
import {
  buildQuadrantDetailSymbol,
  createQuadrantSearchState,
  findQuadrantStockByCode,
  getQuadrantSearchEntry,
  searchQuadrantStocks,
  updateQuadrantSearchEntry,
} from '../lib/quadrant-search'

const QuadrantChart = dynamic(() => import('../components/QuadrantChart'), { ssr: false })
const RankingPanel = dynamic(() => import('../components/RankingPanel'), { ssr: false })

const POLL_MS = 5000
const MARKET_OVERVIEW_POLL_MS = 5000

export default function LiveTradingOverviewPage() {
  const { isLoggedIn, openAuthModal, ready, user } = useAuth()
  const [watchlist, setWatchlist] = useState({ items: [], active_symbol: '', session_state: 'idle' })
  const [snapshots, setSnapshots] = useState([])
  const [marketOverviewA, setMarketOverviewA] = useState(null)
  const [marketOverviewHK, setMarketOverviewHK] = useState(null)
  const [symbolInput, setSymbolInput] = useState('')
  const [nameInput, setNameInput] = useState('')
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState('')
  const [errorNeedsLogin, setErrorNeedsLogin] = useState(false)
  const [lastUpdateAt, setLastUpdateAt] = useState('')
  const [signalConfigMap, setSignalConfigMap] = useState({})
  const [quadrantData, setQuadrantData] = useState(null)
  const [quadrantLoading, setQuadrantLoading] = useState(false)
  const [quadrantExchange, setQuadrantExchange] = useState('ASHARE') // 'ASHARE' | 'HKEX'
  const [rankingData, setRankingData] = useState(null)
  const [rankingLoading, setRankingLoading] = useState(false)
  const [rankingExchange, setRankingExchange] = useState('ASHARE') // independent tab state for ranking
  const [quadrantSearchState, setQuadrantSearchState] = useState(() => createQuadrantSearchState())

  const privateAccessReady = ready && isLoggedIn

  const resetPrivateState = useCallback(() => {
    setWatchlist({ items: [], active_symbol: '', session_state: 'idle' })
    setSnapshots([])
    setSignalConfigMap({})
    setQuadrantData(null)
    setError('')
    setErrorNeedsLogin(false)
    setLastUpdateAt('')
  }, [])

  const updateError = (nextError, nextNeedsLogin = false) => {
    setError(nextError)
    setErrorNeedsLogin(nextNeedsLogin)
  }

  const applyRequestError = (err, fallbackText) => {
    updateError(err.message || fallbackText, isAuthRequiredError(err))
  }

  // Build snapshot lookup by symbol
  const snapshotBySymbol = useMemo(() => {
    const map = {}
    snapshots.forEach((s) => {
      if (s?.symbol) map[s.symbol] = s
    })
    return map
  }, [snapshots])

  const currentQuadrantSearch = useMemo(
    () => getQuadrantSearchEntry(quadrantSearchState, quadrantExchange),
    [quadrantSearchState, quadrantExchange]
  )

  const currentQuadrantStocks = quadrantData?.all_stocks || []

  const currentQuadrantSearchResults = useMemo(
    () => searchQuadrantStocks(currentQuadrantStocks, currentQuadrantSearch.query, quadrantExchange),
    [currentQuadrantStocks, currentQuadrantSearch.query, quadrantExchange]
  )

  const highlightedQuadrantStock = useMemo(
    () => findQuadrantStockByCode(currentQuadrantStocks, currentQuadrantSearch.selectedCode, quadrantExchange),
    [currentQuadrantStocks, currentQuadrantSearch.selectedCode, quadrantExchange]
  )

  const loadWatchlist = async () => {
    const data = await requestJson('/api/live/watchlist')
    const nextState = {
      items: data.items || [],
      active_symbol: data.active_symbol || '',
      session_state: data.session_state || 'idle',
    }
    setWatchlist(nextState)
    return nextState
  }

  const loadSnapshots = async () => {
    const data = await requestJson('/api/live/watchlist/snapshots')
    const items = Array.isArray(data?.items) ? data.items : []
    setSnapshots(items)
    setLastUpdateAt(new Date().toISOString())
    return items
  }

  const loadSignalConfigs = async () => {
    try {
      const data = await requestJson('/api/signal-configs')
      const items = Array.isArray(data?.items) ? data.items : []
      const map = {}
      for (const cfg of items) {
        if (cfg?.symbol) map[cfg.symbol] = cfg
      }
      setSignalConfigMap(map)
    } catch {
      // Signal config loading is non-critical for overview
    }
  }

  const loadQuadrant = async (watchlistSymbols = [], exchange = 'ASHARE') => {
    try {
      setQuadrantLoading(true)
      const params = new URLSearchParams()
      if (exchange === 'HKEX') params.set('exchange', 'HKEX')
      // ASHARE is the default, no need to send param
      if (watchlistSymbols.length > 0) params.set('watchlist_symbols', watchlistSymbols.join(','))
      const qs = params.toString()
      const data = await requestJson(`/api/quadrant${qs ? `?${qs}` : ''}`)
      setQuadrantData(data)
    } catch {
      // Quadrant loading is non-critical
    } finally {
      setQuadrantLoading(false)
    }
  }

  const loadRanking = async (exchange) => {
    try {
      setRankingLoading(true)
      const params = new URLSearchParams()
      params.set('limit', '20')
      if (exchange && exchange !== 'ASHARE') {
        params.set('exchange', exchange)
      }
      // ASHARE 不传 exchange，走后端默认值 (SSE+SZSE)
      const data = await requestJson(`/api/quadrant/ranking?${params.toString()}`)
      setRankingData(data)
    } catch {
      // Ranking loading is non-critical
    } finally {
      setRankingLoading(false)
    }
  }

  const handleRankingExchangeChange = (newExchange) => {
    if (newExchange === rankingExchange) return
    setRankingExchange(newExchange)
    loadRanking(newExchange)
  }

  const loadMarketOverview = async () => {
    const [aRes, hkRes] = await Promise.allSettled([
      requestJson('/api/live/market/overview?exchange=SSE'),
      requestJson('/api/live/market/overview'),
    ])
    if (aRes.status === 'fulfilled') setMarketOverviewA(aRes.value)
    if (hkRes.status === 'fulfilled') setMarketOverviewHK(hkRes.value)
  }

  const loadPrivateData = async ({ bootstrap = false } = {}) => {
    try {
      if (bootstrap) {
        const wl = await loadWatchlist()
        loadSignalConfigs()
        // Load quadrant with watchlist symbols filtered by current exchange
        const symbols = (wl?.items || []).map((i) => i.symbol)
        const filteredSymbols = quadrantExchange === 'HKEX'
          ? symbols.filter((s) => s.toUpperCase().endsWith('.HK'))
          : symbols.filter((s) => !s.toUpperCase().endsWith('.HK'))
        loadQuadrant(filteredSymbols, quadrantExchange)
      }
      await loadSnapshots()
      updateError('')
    } catch (err) {
      applyRequestError(err, '实时数据刷新失败')
    }
  }

  const loadPublicData = async () => {
    try {
      await loadMarketOverview()
    } catch (err) {
      // Market overview failure is non-critical
    }
  }

  // Bootstrap
  useEffect(() => {
    if (!ready) return
    loadPublicData()
    loadRanking(rankingExchange)
    if (privateAccessReady) {
      loadPrivateData({ bootstrap: true })
    } else {
      resetPrivateState()
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [ready, privateAccessReady])

  // Polling
  useEffect(() => {
    if (!ready) return
    const timer = setInterval(() => {
      loadPublicData()
      if (privateAccessReady) {
        loadPrivateData()
      }
    }, POLL_MS)
    return () => clearInterval(timer)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [ready, privateAccessReady])

  useEffect(() => {
    if (!currentQuadrantSearch.selectedCode) return
    if (quadrantLoading) return
    if (currentQuadrantStocks.length === 0) return
    if (highlightedQuadrantStock) return

    setQuadrantSearchState((prev) => updateQuadrantSearchEntry(prev, quadrantExchange, { selectedCode: '' }))
  }, [
    currentQuadrantSearch.selectedCode,
    currentQuadrantStocks.length,
    highlightedQuadrantStock,
    quadrantExchange,
    quadrantLoading,
  ])

  const handleAddWatch = async (event) => {
    event.preventDefault()
    setSubmitting(true)
    updateError('')
    try {
      await requestJson('/api/live/watchlist', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ symbol: symbolInput, name: nameInput }),
      })
      setSymbolInput('')
      setNameInput('')
      await loadWatchlist()
      await loadSnapshots()
    } catch (err) {
      applyRequestError(err, '添加关注失败')
    } finally {
      setSubmitting(false)
    }
  }

  const handleDelete = async (symbol) => {
    updateError('')
    try {
      await requestJson(`/api/live/watchlist/${encodeURIComponent(symbol)}`, { method: 'DELETE' })
      await loadWatchlist()
      await loadSnapshots()
    } catch (err) {
      applyRequestError(err, '删除关注失败')
    }
  }

  const handleOpenDetail = (symbol) => {
    window.open(`/live-trading/${encodeURIComponent(symbol)}`, '_blank')
  }

  const handleQuadrantSearchChange = useCallback((query) => {
    setQuadrantSearchState((prev) => updateQuadrantSearchEntry(prev, quadrantExchange, {
      query,
      selectedCode: '',
    }))
  }, [quadrantExchange])

  const handleQuadrantSearchSelect = useCallback((stock) => {
    if (!stock?.c) return
    setQuadrantSearchState((prev) => updateQuadrantSearchEntry(prev, quadrantExchange, {
      query: stock.n || stock.c,
      selectedCode: stock.c,
    }))
  }, [quadrantExchange])

  const handleQuadrantSearchClear = useCallback(() => {
    setQuadrantSearchState((prev) => updateQuadrantSearchEntry(prev, quadrantExchange, {
      query: '',
      selectedCode: '',
    }))
  }, [quadrantExchange])

  const handleOpenHighlightedQuadrantDetail = useCallback(() => {
    if (!highlightedQuadrantStock?.c) return
    const symbol = buildQuadrantDetailSymbol(highlightedQuadrantStock.c, quadrantExchange)
    if (!symbol) return
    window.open(`/live-trading/${symbol}`, '_blank')
  }, [highlightedQuadrantStock, quadrantExchange])

  const handleQuadrantExchangeChange = async (newExchange) => {
    if (newExchange === quadrantExchange) return
    setQuadrantExchange(newExchange)
    setQuadrantData(null)
    // Reload quadrant with new exchange
    const symbols = (watchlist.items || []).map((i) => i.symbol)
    const filteredSymbols = newExchange === 'HKEX'
      ? symbols.filter((s) => s.toUpperCase().endsWith('.HK'))
      : symbols.filter((s) => !s.toUpperCase().endsWith('.HK'))
    await loadQuadrant(filteredSymbols, newExchange)
  }

  const sortedWatchlist = useMemo(() => {
    return [...(watchlist.items || [])]
  }, [watchlist.items])

  return (
    <div className="space-y-6">
      <Head>
        <title>行情看板 — 卧龙AI量化交易台</title>
        <meta name="description" content="卧龙AI量化交易台行情看板 — 实时 A 股/港股行情、四象限风险全景图、关注池管理。支持 AI 个股智能诊断与技术指标分析。" />
        <link rel="canonical" href="https://wolongtrader.top/live-trading" />
      </Head>
      <section className="rounded-2xl border border-border bg-card p-6">
        <h1 className="text-2xl font-semibold tracking-tight">行情看板</h1>
        <p className="mt-2 text-sm leading-7 text-white/60">
          关注池股票概览，点击卡片可在新标签页打开独立的实时详情页。
        </p>
      </section>

      {/* Market overview — compact index bar */}
      <section className="rounded-2xl border border-border bg-card px-5 py-3">
        <div className="flex flex-wrap items-center gap-x-5 gap-y-2">
          <span className="text-xs font-medium text-white/40">大盘</span>
          {[...(marketOverviewA?.indexes || []), ...(marketOverviewHK?.indexes || [])].map((index) => (
            <div key={index.code} className="flex items-baseline gap-1.5">
              <span className="text-xs text-white/55">{formatMarketIndexTitle(index.name, index.code)}</span>
              <span className="text-sm font-semibold tabular-nums text-white">{formatNumber(index.last, 2)}</span>
              <span className={`text-xs font-medium tabular-nums ${index.change_rate >= 0 ? 'text-rose-300' : 'text-emerald-300'}`}>
                {formatPercent(index.change_rate)}
              </span>
            </div>
          ))}
          {(marketOverviewA?.indexes || []).length === 0 && (marketOverviewHK?.indexes || []).length === 0 && (
            <span className="text-xs text-white/30 animate-pulse">加载中...</span>
          )}
        </div>
      </section>

      {/* Quadrant Analysis */}
      <section className="rounded-2xl border border-border bg-card p-5">
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div>
            <h3 className="text-base font-semibold text-white">风险机会全景图<span className="ml-2 inline-block rounded bg-white/8 px-1.5 py-0.5 text-[11px] font-normal text-white/45">{quadrantExchange === 'HKEX' ? '港股' : 'A 股'}</span></h3>
            {quadrantData?.meta?.computed_at && (
              <div className="mt-1 flex items-center gap-2 text-xs text-white/50">
                <span>数据日期：{formatDateTime(quadrantData.meta.computed_at)}</span>
                {quadrantData.meta.total_count > 0 && <span>· {quadrantData.meta.total_count.toLocaleString()} 只</span>}
                {(() => {
                  if (!quadrantData.meta.computed_at) return null
                  const daysDiff = Math.floor((Date.now() - new Date(quadrantData.meta.computed_at).getTime()) / (1000 * 60 * 60 * 24))
                  if (daysDiff > 3) {
                    return <span className="rounded bg-amber-500/15 px-1.5 py-0.5 text-amber-200">数据已过期（{daysDiff} 天前）</span>
                  }
                  return null
                })()}
              </div>
            )}
          </div>
          {/* Exchange Tab Switch */}
          <div className="flex items-center gap-1 rounded-lg bg-black/20 p-0.5">
            {[
              { key: 'ASHARE', label: 'A 股' },
              { key: 'HKEX', label: '港股' },
            ].map((tab) => (
              <button
                key={tab.key}
                type="button"
                onClick={() => handleQuadrantExchangeChange(tab.key)}
                className={`rounded-md px-3 py-1 text-xs font-medium transition ${
                  quadrantExchange === tab.key
                    ? 'bg-primary text-black'
                    : 'text-white/55 hover:text-white/80 hover:bg-white/[0.05]'
                }`}
              >
                {tab.label}
              </button>
            ))}
          </div>
        </div>

        <div className="mt-4">
          <QuadrantSearchBox
            market={quadrantExchange}
            query={currentQuadrantSearch.query}
            results={currentQuadrantSearchResults}
            selectedCode={currentQuadrantSearch.selectedCode}
            disabled={!quadrantData || currentQuadrantStocks.length === 0}
            onQueryChange={handleQuadrantSearchChange}
            onSelect={handleQuadrantSearchSelect}
            onClear={handleQuadrantSearchClear}
          />
        </div>

        {quadrantLoading && !quadrantData ? (
          <div className="mt-6 flex items-center justify-center py-12 text-sm text-white/40">
            <span className="animate-pulse">加载四象限数据...</span>
          </div>
        ) : quadrantData && quadrantData.all_stocks && quadrantData.all_stocks.length > 0 ? (
          <>
            <div className="mt-4 w-full">
              <QuadrantChart
                allStocks={quadrantData.all_stocks}
                watchlist={quadrantData.watchlist_details || []}
                market={quadrantExchange}
                highlightCode={currentQuadrantSearch.selectedCode}
                autoOpenTooltip={Boolean(currentQuadrantSearch.selectedCode)}
              />
            </div>

            {highlightedQuadrantStock && (
              <div className="mt-4 flex flex-col gap-3 rounded-2xl border border-primary/20 bg-primary/5 px-4 py-3 md:flex-row md:items-center md:justify-between">
                <div className="min-w-0">
                  <div className="flex flex-wrap items-center gap-2 text-sm text-white">
                    <span className="font-semibold text-primary">已定位</span>
                    <span className="font-medium">{highlightedQuadrantStock.n}</span>
                    <span className="font-mono text-xs text-white/45">{highlightedQuadrantStock.c}</span>
                    <span className={`rounded-full px-2 py-0.5 text-[11px] ${quadrantBadgeClass(highlightedQuadrantStock.q)}`}>
                      {highlightedQuadrantStock.q}区
                    </span>
                  </div>
                  <div className="mt-1 text-xs text-white/55">
                    机会 {Number(highlightedQuadrantStock.o || 0).toFixed(1)} / 风险 {Number(highlightedQuadrantStock.r || 0).toFixed(1)}
                  </div>
                </div>
                <div className="flex shrink-0 items-center gap-2">
                  <button
                    type="button"
                    onClick={handleOpenHighlightedQuadrantDetail}
                    className="inline-flex items-center justify-center rounded-xl border border-primary/35 bg-primary/10 px-3 py-2 text-xs font-medium text-primary transition hover:bg-primary/15"
                  >
                    查看详情
                  </button>
                  <button
                    type="button"
                    onClick={handleQuadrantSearchClear}
                    className="inline-flex items-center justify-center rounded-xl border border-white/10 px-3 py-2 text-xs text-white/60 transition hover:bg-white/[0.05] hover:text-white/85"
                  >
                    清除高亮
                  </button>
                </div>
              </div>
            )}

            {/* Summary stats */}
            <div className="mt-4 space-y-3">
              <div className="flex flex-wrap gap-3 text-xs">
                <span className="inline-flex items-center gap-1.5 rounded-lg bg-emerald-500/10 px-2.5 py-1 text-emerald-300">
                  <span className="inline-block h-2 w-2 rounded-full bg-emerald-400" />
                  机会区 {quadrantData.summary?.opportunity_zone || 0}
                </span>
                <span className="inline-flex items-center gap-1.5 rounded-lg bg-amber-500/10 px-2.5 py-1 text-amber-300">
                  <span className="inline-block h-2 w-2 rounded-full bg-amber-400" />
                  拥挤区 {quadrantData.summary?.crowded_zone || 0}
                </span>
                <span className="inline-flex items-center gap-1.5 rounded-lg bg-rose-500/10 px-2.5 py-1 text-rose-300">
                  <span className="inline-block h-2 w-2 rounded-full bg-rose-400" />
                  泡沫区 {quadrantData.summary?.bubble_zone || 0}
                </span>
                <span className="inline-flex items-center gap-1.5 rounded-lg bg-white/5 px-2.5 py-1 text-white/60">
                  <span className="inline-block h-2 w-2 rounded-full bg-white/40" />
                  防御区 {quadrantData.summary?.defensive_zone || 0}
                </span>
                <span className="inline-flex items-center gap-1.5 rounded-lg bg-blue-500/10 px-2.5 py-1 text-blue-300">
                  <span className="inline-block h-2 w-2 rounded-full bg-blue-400" />
                  中性区 {quadrantData.summary?.neutral_zone || 0}
                </span>
              </div>

              {/* Watchlist details text summary */}
              {(quadrantData.watchlist_details || []).length > 0 && (
                <div className="rounded-xl border border-border/60 bg-black/20 p-3">
                  <div className="text-xs font-medium text-white/50">我的关注（{quadrantData.watchlist_details.length} 只）</div>
                  <div className="mt-2 space-y-1">
                    {quadrantData.watchlist_details.map((w) => (
                      <div key={w.code} className="flex items-center gap-2 text-xs">
                        <span className={`inline-block h-2 w-2 rounded-full ${
                          w.quadrant === '机会' ? 'bg-emerald-400' :
                          w.quadrant === '拥挤' ? 'bg-amber-400' :
                          w.quadrant === '泡沫' ? 'bg-rose-400' :
                          w.quadrant === '防御' ? 'bg-white/40' :
                          'bg-blue-400'
                        }`} />
                        <span className="font-medium text-white/80">{w.name}</span>
                        <span className="text-white/40">—</span>
                        <span className="text-white/60">{w.quadrant}区</span>
                        <span className="text-white/40">（机会 {w.opportunity.toFixed(0)} / 风险 {w.risk.toFixed(0)}）</span>
                      </div>
                    ))}
                  </div>
                </div>
              )}
            </div>
          </>
        ) : (
          <div className="mt-4 rounded-xl border border-dashed border-border px-4 py-8 text-center text-sm text-white/40">
            {quadrantData?.all_stocks?.length === 0
              ? '四象限数据尚未计算，请等待凌晨定时任务完成。'
              : '加载四象限数据中...'}
          </div>
        )}
      </section>

      {/* AI Ranking (卧龙AI精选) */}
      <RankingPanel
        items={rankingData?.items}
        meta={rankingData?.meta}
        loading={rankingLoading}
        exchange={rankingExchange}
        onExchangeChange={handleRankingExchangeChange}
      />

      {error ? (
        <div className="rounded-xl border border-rose-400/40 bg-rose-500/10 px-4 py-3 text-sm text-rose-200">
          <div>{error}</div>
          {errorNeedsLogin ? (
            <button
              type="button"
              onClick={() => openAuthModal('login', '行情看板相关操作需要登录后才能继续。')}
              className="mt-2 inline-flex rounded-lg border border-rose-300/40 px-2.5 py-1 text-xs text-rose-100 transition hover:bg-rose-500/15"
            >
              去登录
            </button>
          ) : null}
        </div>
      ) : null}

      {privateAccessReady ? (
        <>
          {/* Add stock form */}
          <section className="rounded-2xl border border-border bg-card p-5">
            <h3 className="text-base font-semibold text-white">添加关注股票</h3>
            <form onSubmit={handleAddWatch} className="mt-3 flex flex-wrap items-end gap-3">
              <input
                value={symbolInput}
                onChange={(e) => setSymbolInput(e.target.value.toUpperCase())}
                placeholder="股票代码，如 00700 或 600519"
                className="w-48 rounded-xl border border-border bg-black/20 px-3 py-2 text-sm text-white outline-none transition focus:border-primary"
              />
              <input
                value={nameInput}
                onChange={(e) => setNameInput(e.target.value)}
                placeholder="备注名称（可选）"
                className="w-40 rounded-xl border border-border bg-black/20 px-3 py-2 text-sm text-white outline-none transition focus:border-primary"
              />
              <button
                type="submit"
                disabled={submitting || !symbolInput.trim()}
                className="rounded-xl bg-primary px-4 py-2 text-sm font-medium text-black transition hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-60"
              >
                {submitting ? '添加中...' : '添加'}
              </button>
            </form>
          </section>

          {/* Stock cards grid */}
          {sortedWatchlist.length === 0 ? (
            <div className="rounded-2xl border border-dashed border-border bg-card px-6 py-12 text-center text-sm text-white/50">
              暂无关注股票，请先在上方添加。
            </div>
          ) : (
            <section className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">
              {sortedWatchlist.map((item) => {
                const snap = snapshotBySymbol[item.symbol]
                const displayName = snap?.name && snap.name !== item.symbol
                  ? snap.name
                  : item.name && item.name !== item.symbol
                    ? item.name
                    : ''
                const changeRate = snap?.change_rate ?? null
                const isUp = changeRate !== null && changeRate > 0
                const isDown = changeRate !== null && changeRate < 0
                const borderAccent = isUp
                  ? 'border-rose-400/30 hover:border-rose-400/50'
                  : isDown
                    ? 'border-emerald-400/30 hover:border-emerald-400/50'
                    : 'border-border hover:border-primary/50'

                return (
                  <div
                    key={item.symbol}
                    className={`group cursor-pointer rounded-2xl border bg-card p-4 transition hover:shadow-lg ${borderAccent}`}
                    onClick={() => handleOpenDetail(item.symbol)}
                    role="button"
                    tabIndex={0}
                    onKeyDown={(e) => { if (e.key === 'Enter') handleOpenDetail(item.symbol) }}
                  >
                    {/* Header */}
                    <div className="flex items-start justify-between gap-2">
                      <div className="min-w-0 flex-1">
                        <div className="truncate text-sm font-semibold text-white">
                          {displayName ? `${displayName}` : item.symbol}
                        </div>
                        <div className="mt-0.5 text-xs text-white/45">
                          {displayName ? item.symbol : ''} · {detectExchangeLabel(item.symbol)}
                        </div>
                      </div>
                      {signalConfigMap[item.symbol]?.is_enabled && (
                        <span className="mt-0.5 inline-flex shrink-0 items-center gap-1 rounded-full border border-emerald-400/30 bg-emerald-500/10 px-2 py-0.5 text-[10px] font-medium text-emerald-300">
                          <span className="inline-block h-1.5 w-1.5 rounded-full bg-emerald-400 animate-pulse" />
                          信号
                        </span>
                      )}
                    </div>

                    {/* Price section */}
                    {snap ? (
                      <div className="mt-3">
                        <div className={`text-xl font-bold tracking-tight ${isUp ? 'text-rose-300' : isDown ? 'text-emerald-300' : 'text-white'}`}>
                          {formatNumber(snap.last_price, snap.last_price >= 100 ? 2 : 3)}
                        </div>
                        <div className="mt-1 flex items-center gap-3 text-xs">
                          <span className={isUp ? 'text-rose-300' : isDown ? 'text-emerald-300' : 'text-white/60'}>
                            {formatPercent(changeRate)}
                          </span>
                          {snap.volume_ratio > 0 && (
                            <span className="text-white/45">量比 {formatNumber(snap.volume_ratio, 2)}</span>
                          )}
                        </div>
                        <div className="mt-2 grid grid-cols-2 gap-x-4 gap-y-1 text-[11px] text-white/50">
                          <div>成交量 <span className="text-white/70">{formatCompact(snap.volume)}</span></div>
                          <div>成交额 <span className="text-white/70">{formatCompact(snap.turnover)}</span></div>
                          <div>振幅 <span className="text-white/70">{formatPercent(snap.amplitude)}</span></div>
                        </div>
                      </div>
                    ) : (
                      <div className="mt-3 text-xs text-white/40">加载中...</div>
                    )}

                    {/* Footer actions */}
                    <div className="mt-3 flex items-center justify-between border-t border-white/5 pt-3">
                      <span className="text-[11px] text-white/40 transition group-hover:text-primary">
                        点击查看详情 →
                      </span>
                      <button
                        type="button"
                        onClick={(e) => {
                          e.stopPropagation()
                          handleDelete(item.symbol)
                        }}
                        className="rounded-lg px-2 py-1 text-[11px] text-rose-300/60 transition hover:bg-rose-500/10 hover:text-rose-300"
                      >
                        删除
                      </button>
                    </div>
                  </div>
                )
              })}
            </section>
          )}

          {lastUpdateAt && (
            <div className="text-right text-xs text-white/35">
              最后更新：{formatDateTime(lastUpdateAt)}
            </div>
          )}
        </>
      ) : (
        <section className="rounded-2xl border border-dashed border-primary/30 bg-primary/10 p-6">
          <div className="flex flex-col gap-4 md:flex-row md:items-start md:justify-between">
            <div className="space-y-2">
              <div className="text-lg font-semibold text-white">
                {ready ? '登录后开启行情看板' : '正在确认账号状态'}
              </div>
              <p className="max-w-2xl text-sm leading-7 text-white/65">
                {ready
                  ? '登录后可管理关注池、查看实时行情快照和独立股票详情页。'
                  : '正在检查你的登录状态，确认后会自动加载数据。'
                }
              </p>
            </div>
            <button
              type="button"
              disabled={!ready}
              onClick={ready ? () => openAuthModal('login', '登录后即可管理关注池与行情看板。') : undefined}
              className="inline-flex shrink-0 items-center justify-center rounded-xl bg-primary px-4 py-2 text-sm font-semibold text-black transition hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-60"
            >
              {ready ? '登录后继续' : '请稍候'}
            </button>
          </div>
        </section>
      )}
    </div>
  )
}

// ── Utility functions ──

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

function formatCompact(value) {
  if (value === null || value === undefined || Number.isNaN(Number(value))) return '--'
  return Number(value).toLocaleString('zh-CN', { maximumFractionDigits: 2 })
}

function formatDateTime(value) {
  if (!value) return '--'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return date.toLocaleString('zh-CN', { hour12: false })
}

function detectExchange(symbol) {
  if (!symbol) return 'HKEX'
  const upper = String(symbol).toUpperCase()
  if (upper.endsWith('.SH')) return 'SSE'
  if (upper.endsWith('.SZ')) return 'SZSE'
  if (upper.endsWith('.HK')) return 'HKEX'
  const digits = upper.replace(/\D/g, '')
  if (digits.length === 6) {
    if (digits[0] === '6') return 'SSE'
    if (digits[0] === '0' || digits[0] === '3') return 'SZSE'
  }
  return 'HKEX'
}

function detectExchangeLabel(symbol) {
  const ex = detectExchange(symbol)
  const labels = { SSE: '沪市', SZSE: '深市', HKEX: '港股' }
  return labels[ex] || ex
}

function quadrantBadgeClass(quadrant) {
  switch (quadrant) {
    case '机会':
      return 'bg-emerald-500/12 text-emerald-300'
    case '拥挤':
      return 'bg-amber-500/12 text-amber-300'
    case '泡沫':
      return 'bg-rose-500/12 text-rose-300'
    case '防御':
      return 'bg-white/8 text-white/70'
    default:
      return 'bg-blue-500/12 text-blue-300'
  }
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
